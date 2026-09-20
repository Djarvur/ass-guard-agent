---
phase: 19-compaction-cache-control
plan: "05"
subsystem: modelrouting
tags: [compaction, config, configoptions, live-apply, tdd]

# Dependency graph
requires:
  - phase: 19-compaction-cache-control plan 19-04
    provides: the compaction engine + Session.SetCompactionSettings (the live-apply landing this plan wires the config onto)
  - phase: 16-acp-wire-foundation plan 16-04
    provides: providerfactory.WriteLayerOption (the atomic 0600 persist-then-apply primitive)
  - phase: 16-acp-wire-foundation plan 16-05
    provides: the ConfigSurface at internal/acpserve/config_surface.go (menu semantics, scope routing, D-09/D-10 guards)
provides:
  - modelrouting Config.Compaction (ThresholdPct/Enabled under the compaction yaml key) with the embedded floor's explicit 80/true defaults (seed copy synced, drift guard green)
  - The compaction configOptions menu entries (compaction-threshold handled, compaction-enabled added, both with _global/ twins — the twelve-entry menu) with effective-value advertisement, validation-first 1..100/on-off typed rejects, atomic per-layer persistence, D-10 idempotent re-push, and live apply through SetCompactionHook → Runner.ApplyCompactionSettings → Session.SetCompactionSettings
  - The session settings source: construction initializes from the modelrouting-loaded effective values (the runtime seam), the context limit resolved from the capability table; the session-side comparison clamps pct into 1..100 defensively
affects: [PAR-01 verification, Phase 20 (/compact rides CompactNow; config keys exist), 25-01 kit extraction ordering]

# Actuals (#2632) — pairs with the plan's estimate (24000) to calibrate future estimates.
actuals:
  tokens: 18800   # chars/4 over the realized diff (75207 diff chars)
  tasks: 2
  commits: 4

tech-stack:
  added: []   # stdlib + existing yaml dep only (T-19-SC accepted)
  patterns:
    - "the floor yaml — not applyDefaults — is the absent-key defaulting site for keys whose out-of-range values must pass through Load untouched (a bool cannot distinguish absent from explicit-false; an int backstop would eat the session clamp's 0→1 case)"
    - "live-apply targets carry the POST-WRITE effective PAIR, never the written id alone — the session's check consumes both settings, so a one-id write resolves the other from the layers"
    - "layer-map scalars: the config writer emits int/bool leaves (never strings) so modelrouting.Load round-trips what WriteLayerOption wrote; the surface's readers tolerate hand-quoted strings"

key-files:
  created: []
  modified:
    - internal/modelrouting/config.go
    - internal/modelrouting/defaults/config.yaml
    - internal/modelrouting/load_test.go
    - internal/defaults/seed/config.yaml
    - internal/providerfactory/config_write_test.go
    - internal/session/compaction.go
    - internal/session/compaction_test.go
    - internal/runtime/runtime.go
    - internal/acpserve/config_surface.go
    - internal/acpserve/config_test.go
    - internal/acpserve/config_surface_lock_test.go
    - internal/acpserve/acp_serve.go

key-decisions:
  - "The embedded floor is the absent-key defaulting site for the compaction keys (defaults/config.yaml carries threshold_pct: 80 / enabled: true): a bool cannot distinguish absent from explicit false at applyDefaults, and an int backstop (<=0 → 80) would contradict the plan's own clamp acceptance — a hand-edited 0 must reach the engine and compare as 1"
  - "The surface's live-apply target is the POST-WRITE effective PAIR (the written id plus the other id resolved through project > global > blob > default): the session's pre-request check consumes both settings, so a one-id write never ships a half pair"
  - "The context limit is resolved INSIDE Runner.ApplyCompactionSettings from the modelrouting capability table (the session model's capabilities.context_window) — deliberately absent from the menu, the config keys, and the hook signature (the resolved discretion item: an override key would fork the measurement baseline)"
  - "Session.SetCompactionSettings keeps 19-04's landed (enabled, thresholdPct, contextLimit) signature — the plan prose's two-parameter shorthand would orphan the context limit; the defensive <=0 → 80 remap moved out of the setter into the comparison (overThreshold clamps 1..100), so the stored value is exactly what the config resolved"
  - "Compaction blob fills are advertisement-only D-10 in-memory defaults (no wire stamp — unlike tier/model's CR-02 blobHook): a client default push never changes what the running session enforces; the operator's live lever is set_config_option"
  - "The offered threshold select set dropped \"off\" (it predates the handler): with 1..100 validation a threshold can no longer be \"off\" — disabling is compaction-enabled's job"

requirements-completed: [PAR-01]

coverage:
  - id: C1
    description: "modelrouting compaction keys: absent keys load the floor's 80/true, explicit values load, marshal round-trip preserves both"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/modelrouting/load_test.go#TestCompaction"
        status: pass
    human_judgment: false
  - id: C2
    description: "Layer-writer round-trip: WriteLayerOption writes then Load resolves (threshold in the global layer, enabled in the project layer); a project write leaves the global file byte-identical"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/providerfactory/config_write_test.go#TestConfigWrite_CompactionRoundTrip"
        status: pass
    human_judgment: false
  - id: C3
    description: "Session settings source: construction over a 60-percent config compares against 60 (not a shadow default), SetCompactionSettings re-targets the next check, and the comparison clamps pct into 1..100 (0 and 500 compare as 1 and 100)"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/session/compaction_test.go#TestCompaction_SettingsFromConfig"
        status: pass
    human_judgment: false
  - id: C4
    description: "Menu wiring: effective-value advertisement through the layer chain, valid threshold/enabled sets persist-then-apply (observed by a recording hook), out-of-range typed rejection preceding any write, the pending branch removed, D-10 idempotent re-push, _global scope routing"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/acpserve/config_test.go#TestCompactionOptions"
        status: pass
    human_judgment: false
  - id: C5
    description: "The effective-value round-trip invariant (the assumption-delta companion): a threshold written through the menu over a live serve is the value the running session's next pre-request check compares against — no fire at a menu-written 95, fire at a menu-written 40, constant 50% usage"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/acpserve/config_test.go#TestCompactionLive_MenuThresholdRoundTrip"
        status: pass
    human_judgment: false
  - id: C6
    description: "Construction initialization end-to-end: a live serve's session compacts at the BOOT defaults (enabled/80%/200K from the loaded floor) with no menu interaction — turn 2 fires at 85% usage"
    requirement: PAR-01
    verification:
      - kind: unit
        ref: "internal/acpserve/config_test.go#TestCompactionLive_BootDefaults"
        status: pass
    human_judgment: false

# Metrics
duration: 36min
completed: 2026-09-06
status: complete
---

# Phase 19 Plan 05: Compaction Config Keys and Menu Wiring Summary

**D-03 landed: compaction.threshold_pct (80) and compaction.enabled (true) are first-class modelrouting config keys and configOptions menu citizens — advertised with effective values, typed-rejected out of range at the wire, persisted atomically per layer, and live-applied to the running session's next pre-request check through the ApplyTurnModel-discipline seam**

## Performance

- **Duration:** 36 min
- **Started:** 2026-09-06T22:32:19Z
- **Completed:** 2026-09-06T23:08:07Z
- **Tasks:** 2 (both RED→GREEN under TDD mode)
- **Files:** 12 (0 created, 12 modified)

## Accomplishments
- `internal/modelrouting/config.go` + `defaults/config.yaml`: `Config.Compaction` (ThresholdPct/Enabled under the `compaction` yaml key, parse/default/round-trip only — Validate stays hands-off, the 16-04 session_tier discipline) with the embedded floor carrying the explicit 80/true defaults; the `internal/defaults/seed/config.yaml` copy synced (drift guard green)
- `internal/session/compaction.go`: the settings holder now stores exactly what the config resolved — the defensive `<=0 → 80` remap moved from `SetCompactionSettings` into the comparison (`clampCompactionPct`: 0 and 500 compare as 1 and 100); `DefaultCompactionThresholdPct` now documents the floor's value (the operative default source is the config)
- `internal/runtime/runtime.go`: the runner-level effective compaction slot (lazily seeded from schedCfg), sessionFor initializing every session's settings from the loaded effective values at construction with the context limit resolved from the capability table for that session's model, and `ApplyCompactionSettings` — the live-apply relay that swaps every live session's settings under its turn mutex (mid-turn Sets land between turns)
- `internal/acpserve/config_surface.go`: compaction-threshold flipped from the pending no-op to the real Set flow and compaction-enabled added (both with `_global/` twins — the twelve-entry menu); validation-first typed rejects (whole 1..100 percentage / two-value switch, with the option key and violation in the error data BEFORE any write), int/bool persistence through WriteLayerOption, effective-value advertisement through project > global > blob fill > default, the D-10 idempotent re-push, unchanged scope routing, the `SetCompactionHook` seam, and deletion of the entire pending machinery (`setPendingLocked`/`isPendingOption`/`phasePendingCompaction`)
- `internal/acpserve/acp_serve.go`: the composition binds `surface.SetCompactionHook(runner.ApplyCompactionSettings)`
- The proof battery: `TestCompaction` (load/round-trip), `TestConfigWrite_CompactionRoundTrip` (both layers + byte-identical global), `TestCompaction_SettingsFromConfig` (settings source + clamp, session package), `TestCompactionOptions` (the six-behavior surface battery), and two live-serve E2Es over a usage-configurable SSE stub — `TestCompactionLive_BootDefaults` (construction init fires at the 80% boot default) and `TestCompactionLive_MenuThresholdRoundTrip` (the assumption-delta companion: no fire at a menu-written 95, fire at a menu-written 40, constant usage)

## Task Commits

Each behavior-adding task followed RED→GREEN with per-plan commits:

1. **Task 1: modelrouting compaction keys with defaults and round-trip** — `1c61b5d` (test, RED) + `5c6bd52` (feat, GREEN)
2. **Task 2: Menu wiring — compaction-threshold handled, compaction-enabled added** — `6abd30d` (test, RED) + `912499b` (feat, GREEN)

## Files Created/Modified
- `internal/modelrouting/config.go` — the CompactionConfig type + Config field
- `internal/modelrouting/defaults/config.yaml` — the explicit compaction defaults block
- `internal/defaults/seed/config.yaml` — the byte-identical seed copy (TestDriftGuard_SchedulingYAML)
- `internal/modelrouting/load_test.go` — TestCompaction (the session_tier round-trip shape, mirrored)
- `internal/providerfactory/config_write_test.go` — TestConfigWrite_CompactionRoundTrip (cross-package, the 16-04 precedent)
- `internal/session/compaction.go` — raw-value setter + clampCompactionPct in the comparison
- `internal/session/compaction_test.go` — TestCompaction_SettingsFromConfig
- `internal/runtime/runtime.go` — the effective-compaction slot, the construction seam, the ApplyCompactionSettings relay, compactionContextLimit
- `internal/acpserve/config_surface.go` — the handler pair, hook, resolution helpers, pending machinery deleted
- `internal/acpserve/config_test.go` — the battery + the two live E2Es + the usage-configurable stub
- `internal/acpserve/config_surface_lock_test.go` — the menu-size pin moved to twelve entries
- `internal/acpserve/acp_serve.go` — the hook binding

## Decisions Made
- **The floor is the defaulting site:** applyDefaults cannot default a bool (absent ≡ false) and an int backstop would contradict the plan's own clamp acceptance (a hand-edited 0 must compare as 1, not silently become 80) — so the absent-key 80/true defaults live in the embedded floor yaml and Load's overlay resolves them; out-of-range values pass through untouched
- **The apply carries the pair:** the surface resolves the post-write effective (enabled, threshold) — a one-id write never ships a half pair to a session that consumes both; the context limit resolves inside the relay (no menu entry, no config key, not a hook parameter — the resolved discretion item)
- **The setter signature follows the landed artifact:** 19-04's `(enabled, thresholdPct, contextLimit)` stands; the plan prose's two-parameter shorthand would orphan the context limit
- **Blob fills stay advertisement-only:** unlike tier/model's CR-02 wire stamp, a compaction blob fill never changes what the running session enforces (a client default push is not operator intent); the advertisement shows it, the engine ignores it, set_config_option is the lever
- **"off" left the threshold's offered set:** with 1..100 validation the old "off" affordance is incoherent — the switch id owns disabling

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Test-file placement followed the landed artifact, not the plan's file list**
- **Found during:** Task 1 RED
- **Issue:** the plan names `internal/modelrouting/config_test.go`, but the repo's session_tier load/round-trip cases live in `load_test.go` (no config_test.go exists in modelrouting)
- **Fix:** the new cases landed in `load_test.go` beside TestSessionTier (the A5 follow-the-landed-artifact fallback the plan itself prescribes for divergences)
- **Files modified:** internal/modelrouting/load_test.go
- **Verification:** go test ./internal/modelrouting/ -run 'Compaction|ConfigWrite' green
- **Committed in:** 1c61b5d

**2. [Rule 3 - Blocking] Files beyond the plan's files_modified list were required by the plan's own behaviors**
- **Found during:** Tasks 1-2
- **Issue:** the must-have truths demand construction initialization ("the session's settings holder initializes from the modelrouting-loaded effective values at construction") and the runner relay ("add the runner relay only if the landed seam requires it" — it does: the surface cannot reach sessions), but files_modified lists neither runtime.go nor acp_serve.go; the defaults seed copy and the lock-test menu pin are likewise downstream obligations
- **Fix:** runtime.go (effective slot + construction seam + ApplyCompactionSettings), acp_serve.go (SetCompactionHook binding), internal/defaults/seed/config.yaml (TestDriftGuard_SchedulingYAML byte-identity), config_surface_lock_test.go (the WR-03 probe's twelve-entry pin)
- **Files modified:** as listed
- **Verification:** the full verify block green; drift guard green
- **Committed in:** 5c6bd52, 912499b (seed in 5c6bd52; lock pin in 6abd30d)

---

**Total deviations:** 2 auto-fixed (both Rule 3 blockers of the follow-the-landed-artifact class)
**Impact on plan:** No scope change — the additions are the plan's own named behaviors; only the file manifest was incomplete.

## Issues Encountered
- **internal/runtime load-flake (pre-existing, out of scope):** `TestAskPark_PromptResponsePrecedesResolution` failed once under the full-suite race run (the documented deferred-items.md family); passes 3/3 in isolation. 19-05's changes touch sessionFor (construction-time settings init, inert until a usage chunk arrives) — the flake predates this plan (19-04 recorded the same failure before any runtime change existed).
- **mise ci lint leg red (pre-existing, out of scope):** the documented repo-wide golangci 2.12↔2.13 exhaustruct_v5 rename drift (2831 findings); vet, CGO_ENABLED=0 build, and the race suite green except the flake above; all files this plan touched carry zero 2.13.2 findings beyond the renamed-linter noise (verified per-package, and against the CI lint output).

## TDD Gate Compliance

Both behavior-adding tasks followed RED→GREEN with committed gates: `test(19-05)` commits 1c61b5d and 6abd30d each precede their `feat(19-05)` GREEN commits 5c6bd52 and 912499b. RED failures verified for the right reasons: undefined `Config.Compaction` (Task 1 — the missing keys, the 19-04 undefined-symbol precedent) and undefined `optCompactionEnabled`/`SetCompactionHook` (Task 2 — the missing menu id and seam). The pre-plan pending-behavior pin (`TestConfigSurface_PendingNoOp_Compaction`) was replaced by the battery in the Task 2 RED commit — its assertions invert with the pending branch's removal, which is the change under test.

## Verification
- `go test -race ./internal/acpserve/ ./internal/modelrouting/ ./internal/providerfactory/ -count=1` — green (94s acpserve full package under race)
- `go test -race ./internal/session/ -run 'TestCompaction' -count=1` — green (the whole battery incl. 19-04's, unaffected by the settings-source change)
- `go vet ./internal/modelrouting/` — clean; `go vet ./...` clean; `CGO_ENABLED=0 go build ./...` clean
- `go test ./internal/acp/ ... -count=1` — the handler layer passes UNMODIFIED (zero internal/acp files touched)
- `go test -race -count=1 ./...` — green except the documented runtime load-flake (isolation-verified 3/3)
- `mise ci` — lint leg red on the pre-existing repo-wide drift (documented); vet/build/test legs green (test modulo the flake)
- The pending-handler log path for compaction-threshold is gone: `grep -c 'setPendingLocked\|isPendingOption\|phasePendingCompaction' internal/acpserve/config_surface.go` = 0
- No context-limit key anywhere: the menu advertises exactly model, tier, permissions.mode, compaction-threshold, compaction-enabled, tombstoneGraceDays + twins (12 entries — pinned by assertFullMenu)

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 19 is COMPLETE: all five plans (19-01..19-05) have SUMMARYs — PAR-01's shared-ID gate releases with this file
- The assumption-delta invariant is pinned from both sides: TestCompactionLive_MenuThresholdRoundTrip goes red if a future phase reintroduces a shadow default between the menu and the engine
- Phase 20's /compact handler rides the existing CompactNow(ctx) — one implementation, two triggers, per D-11

## Self-Check: PASSED

- Files exist: all 12 key-files.modified present on disk and in the 19-05 commit range (1c61b5d..912499b)
- Commits 1c61b5d, 5c6bd52, 6abd30d, 912499b all present on gsd/v1.2-claude-code-parity
- All six coverage entries verified pass by the runs above

---
*Phase: 19-compaction-cache-control*
*Completed: 2026-09-06*
