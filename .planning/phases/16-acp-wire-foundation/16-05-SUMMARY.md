---
phase: 16-acp-wire-foundation
plan: 05
subsystem: acp-config-wire
tags: [acp, config-options, set-config-option, yaml-layers, scope-routing, live-apply, tdd]
requires:
  - "16-03 emitter/registry spine (EmitterHandle.Notify foreground route, Server options)"
  - "16-04 providerfactory.WriteLayerOption (atomic 0600 layer writer) + modelrouting.Load/DeepMerge precedence"
provides:
  - "v1 wire shapes: ConfigOptionFrame/ConfigOptionValue (id/name/category/type=select + currentValue + options), session/set_config_option handler, config_option_update frame (KindConfigOptionUpdate)"
  - "acp.ConfigSurface interface + WithConfigSurface ServerOption + Server.NotifyConfigOptions (out-of-band update frame through the emitter foreground lane)"
  - "acpserve.ConfigSurface: eight-entry menu from real layers, effective-value resolution (D-11), _global/ scope routing (D-08), _meta fills-unset blob (D-10), D-10 idempotence guard on the set channel, serialized mutations"
  - "Runner.ApplyTurnModel + Session.SetTurnModel: the serialized live-apply seam — editor model/tier switches reach the very next provider request"
affects:
  - "16-06 simulator (drives the whole surface as one story; live-editor confirmation)"
  - "Phase 17 (permissions.mode handler registers behind the locked shape)"
  - "Phase 19 (compaction-threshold handler) / Phase 18 (load/resume reuse configOptionsFor) / Phase 20 (/model tops the chain)"
tech-stack:
  added: [] # stdlib + existing yaml.v3/modelrouting/providerfactory only
  patterns:
    - "wire truth over plan prose: the set REQUEST field is configId (v1 SetSessionConfigOptionRequest) while the ADVERTISEMENT key is id — both pinned against the fetched schema"
    - "surface owns menu semantics, handler only relays: D-09 violations and persist failures arrive as acp-typed error classes and translate to distinct JSON-RPC codes"
    - "fills-unset explicitness boundary: a layer FILE is operator config the blob never overrides; the embedded floor is not — the blob beats the floor, in-memory only"
    - "mid-turn mutation lands BETWEEN turns: ApplyTurnModel re-stamps under the per-session turn mutex (the 12-07 queue semantics reused as the no-torn-stamp gate)"
key-files:
  created:
    - internal/acpserve/config_surface.go
    - internal/acpserve/config_test.go
    - internal/acp/handlers_test.go
    - internal/runtime/apply_model_test.go
    - internal/session/turn_model_test.go
  modified:
    - internal/acp/types.go
    - internal/acp/handlers.go
    - internal/acp/server.go
    - internal/acpserve/acp_serve.go
    - internal/runtime/runtime.go
    - internal/session/session.go
key-decisions:
  - "v1 request field is configId (SetSessionConfigOptionRequest) while the advertisement option key is id — both verbatim from the fetched schema (the plan prose's 'optionId' normalized to wire truth, Pitfall-7 discipline)"
  - "ConfigSurface frozen minimal (Options/Set/ApplyBlobDefaults); acp.Server.NotifyConfigOptions owns the config_option_update frame through the emitter foreground lane — the sessionless initialize-blob update rides an empty sessionId (nothing to re-render, set still carried)"
  - "D-10 explicitness boundary: only LAYER FILES are operator config the blob never overrides; the embedded floor loses to the blob — fills-unset is in-memory only, never persisted, never promoted by a re-push (idempotence guard on the set channel)"
  - "model option writes tiers.<current tier>.model, tier writes session_tier; _global/ prefix addresses the global layer, project stays the default target (D-08/A7)"
  - "live apply rides Runner.ApplyTurnModel under the per-session turn mutex — mid-turn Sets land between turns (no torn stamp); cross-provider tier/model targets degrade loudly with the model unchanged (resolveSubagentModel precedent)"
requirements-completed: [ACP-08]
duration: 49 min
completed: 2026-08-27
status: complete
estimate:
  tokens: 46000
  raw_tokens: 46000
  tasks: 3
  confidence: low
actuals:
  tokens: 27048 # chars/4 over the realized diff (108,193 chars, 11 files, 2890 insertions)
  tasks: 3
  commits: 7
coverage:
  - id: D1
    description: "Advertisement: session/new (and initialize) carry the eight-entry v1.2 menu, each currentValue the surface's effective resolution at response time (D-06/D-11); loadSession stays false"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acp/handlers_test.go#TestConfigAdvertise"
        status: pass
    human_judgment: false
  - id: D2
    description: "set_config_option core: valid set returns the FULL refreshed set with the exact Set call recorded; unknown id / invalid value typed -32602 with optionId+violation data; surface write failure is a DISTINCT typed class with the apply path provably unreached; pending ids are logged no-ops with unchanged currentValue (and invalid pending values still typed-rejected); no-surface setups degrade to the typed not-available error"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acp/handlers_test.go#TestSetConfigOptionValid"
        status: pass
      - kind: unit
        ref: "tests/internal/acp/handlers_test.go#TestSetConfigOptionUnknownID"
        status: pass
      - kind: unit
        ref: "tests/internal/acp/handlers_test.go#TestSetConfigOptionInvalidValue"
        status: pass
      - kind: unit
        ref: "tests/internal/acp/handlers_test.go#TestSetConfigOptionWriteFailure"
        status: pass
      - kind: unit
        ref: "tests/internal/acp/handlers_test.go#TestSetConfigOptionPendingNoOp"
        status: pass
      - kind: unit
        ref: "tests/internal/acp/handlers_test.go#TestSetConfigOptionPendingInvalid"
        status: pass
      - kind: unit
        ref: "tests/internal/acp/handlers_test.go#TestSetConfigOptionNoSurface"
        status: pass
    human_judgment: false
  - id: D3
    description: "ConfigSurface menu from real layers: defaults-only advertises the embedded floor's models/tiers with resolved currentValues; an explicit layer value wins the advertisement"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestConfigSurface_MenuDefaults"
        status: pass
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestConfigSurface_ExplicitLayerEffectiveValues"
        status: pass
    human_judgment: false
  - id: D4
    description: "_meta blob (D-10): explicit-file-wins in BOTH directions, fills-unset applies in-memory only (no layer file ever appears), exactly one config_option_update with the full set follows an application that moved a value, unknown keys retained byte-identical in the serve state"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestMetaBlob_ExplicitFileWins"
        status: pass
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestMetaBlob_FillsUnsetInMemory"
        status: pass
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestMetaBlob_UnknownKeysRetainedVerbatim"
        status: pass
    human_judgment: false
  - id: D5
    description: "Scope routing (D-08): _global/ writes the global layer, the un-prefixed default writes the project layer, the untouched counterpart stays byte-identical"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestScopeRouting_GlobalPrefixWritesGlobalLayer"
        status: pass
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestScopeRouting_DefaultWritesProjectLayer"
        status: pass
    human_judgment: false
  - id: D6
    description: "Persistence (D-07 write half): a valid set persists into the addressed layer and the REAL modelrouting.Load resolves the written value; blob application + interleaved sets serialize (parseable file, deterministic per-key winner, memory==disk)"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestConfigSurface_SetPersistsThroughLoader"
        status: pass
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestConfigSurface_ConcurrentMutation"
        status: pass
    human_judgment: false
  - id: D7
    description: "D-10 idempotence guard on the set channel: a re-push equal to the effective value — blob-derived or file-explicit — writes nothing, changes no overlay, emits no update, and logs exactly one line; the layer file stays byte-identical"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestSetIdempotent_BlobDerivedEffective"
        status: pass
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestSetIdempotent_ExplicitFileEffective"
        status: pass
    human_judgment: false
  - id: D8
    description: "Live apply over the full Run composition: set_config_option(model) and a same-provider tier switch change the VERY NEXT provider request (request_shaped fingerprints), the surface->runner->server wiring needs no manual step"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestLiveModelApply"
        status: pass
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestTierSwitch"
        status: pass
    human_judgment: false
  - id: D9
    description: "Turn serialization: a Set issued while a slow provider holds a request open waits — the in-flight request keeps its model, the next carries the new one (no torn stamp); sessions created post-change stamp the effective model at construction"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestLiveModelApply_MidTurn"
        status: pass
      - kind: unit
        ref: "tests/internal/runtime/apply_model_test.go#TestApplyTurnModel"
        status: pass
      - kind: unit
        ref: "tests/internal/runtime/apply_model_test.go#TestApplyTurnModel_StampsFutureSessions"
        status: pass
    human_judgment: false
  - id: D10
    description: "Cross-provider tier switch: persist succeeds, one loud structured warning, live model unchanged"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/acpserve/config_test.go#TestTierSwitch_CrossProvider"
        status: pass
    human_judgment: false
  - id: D11
    description: "Session.SetTurnModel stamps the profile copy the Shaper reads at every Stream"
    requirement: ACP-08
    verification:
      - kind: unit
        ref: "tests/internal/session/turn_model_test.go#TestSetTurnModel"
        status: pass
    human_judgment: false
  - id: D12
    description: "Live-editor confirmation: Zed's settings UI shows the agent's options with true current values and an editor switch changes the next request"
    verification: []
    human_judgment: true
    rationale: "Per the plan's own verification note: the 16-06 simulator drives the whole surface as one story and live-editor confirmation happens there (the 15-07 live-check pattern). In-repo proof is the deterministic wire + serve test set above."
---

# Phase 16 Plan 05: ACP-08 Config Wire Surface + Live Apply Summary

**The full ACP-08 agent half is live: Zed's session/new carries the eight-option menu with true effective values, `session/set_config_option` validates typed, persists atomically into the operator's project/global layers (with a D-10 idempotence guard against redundant default re-pushes), reads the initialize _meta blob as fills-unset in-memory defaults, and an editor's model/tier switch reaches the very next provider request through a serialized runner seam.**

## Performance

- **Duration:** 49 min
- **Started:** 2026-08-27T16:40:13Z
- **Completed:** 2026-08-27T17:29:18Z
- **Tasks:** 3 (all TDD — RED and GREEN committed separately)
- **Files:** 11 (5 created, 6 modified)

## Accomplishments

- **Wire half (Task 1):** `ConfigOptionFrame`/`ConfigOptionValue` in verbatim v1 names, the shared `configOptionsFor` advertisement builder feeding initialize + session/new (ready for Phase 18's load/resume), `handleSetConfigOption` relaying to the injected `ConfigSurface` with D-09 violations as `-32602 {optionId, violation}` data and persist failures as a distinct internal class, `loadSession` still honestly false, and the no-surface degrade (omitted advertisement + typed not-available error) keeping every acpserve-less setup working.
- **Surface half (Task 2):** the `acpserve.ConfigSurface` builds the menu from the REAL modelrouting layers (project > global > embedded floor), resolves effective currentValues through the resolver (D-11), routes `_global/`-prefixed ids to the global layer with the project as default (D-08/A7), applies the initialize `_meta` blob as fills-unset IN-MEMORY defaults that never persist and never override a layer-file value (D-10) — retaining unknown keys byte-identical — guards the set channel against idempotent re-pushes (a redundant Zed default can neither churn the operator's file nor promote a blob-derived value into persisted config), and serializes every mutation behind one mutex (no torn YAML, memory==disk).
- **Live apply (Task 3):** `Runner.ApplyTurnModel` re-stamps every live session UNDER the session's turn mutex — a mid-turn Set waits, the in-flight request keeps its model, the next request carries the new one — and `sessionFor` stamps the effective model at construction so future sessions follow. The Run composition constructs the surface at the startup junction (degrading to project-only loudly if the global path fails), binds its apply hook to the runner and its notification to `srv.NotifyConfigOptions` (the config_option_update frame through the emitter foreground lane).
- **Pending honesty (D-05):** `permissions.mode` (Phase 17) and `compaction-threshold` (Phase 19) are advertised, validated, accepted as one-structured-line pending no-ops — never errors, never silently discarded.

## TDD Gate Compliance

| Task | RED | GREEN | REFACTOR | Status |
|------|-----|-------|----------|--------|
| Task 1 (wire shapes + handler) | ✓ `6c899cf` (build-failure — surface API absent) | ✓ `9d53e7a` | — (lint-convention passes rode GREEN) | Pass |
| Task 2 (ConfigSurface) | ✓ `007b6b1` (build-failure — ConfigSurface absent) | ✓ `eab5136` | — | Pass |
| Task 3 (live apply seam) | ✓ `d3a5aed` (build-failure session/runtime + behavioral serve RED) | ✓ `a34d9e1` | — | Pass |

## Task Commits

1. **Task 1 RED** — `6c899cf` (test): failing config wire surface tests
2. **Task 1 GREEN** — `9d53e7a` (feat): wire shapes, advertisement, set_config_option core
3. **Task 2 RED** — `007b6b1` (test): failing ConfigSurface implementation tests
4. **Task 2 GREEN** — `eab5136` (feat): scope routing, _meta blob, idempotence
5. **Task 3 RED** — `d3a5aed` (test): failing live-apply seam + serve integration tests
6. **Task 3 GREEN** — `a34d9e1` (feat): runner seam, turn serialization, Run composition
7. **Plan metadata** — docs commit (this close-out)

## Verification Results

- `go test -race ./internal/acp/ -run 'TestSetConfigOption|TestConfigAdvertise' -count=1 && go test ./internal/acp/ -count=1` — green
- `go test -race ./internal/acpserve/ -run 'TestConfigSurface|TestMetaBlob|TestScopeRouting|TestSetIdempotent' -count=1` — green (12 tests)
- `go test -race ./internal/acpserve/ -run 'TestLiveModelApply|TestTierSwitch' -count=1 && go test -race ./internal/runtime/ -run TestApplyTurnModel -count=1 && go test ./internal/session/ -count=1` — green
- `go test -race ./internal/acp/ ./internal/acpserve/ ./internal/runtime/ ./internal/session/ -count=1` — green
- `mise ci` (vet + golangci-lint strict + build + `go test -race ./...`) — exit 0, full repo
- `grep -c 'session/set_config_option' internal/acp/handlers.go` = 3 (≥ 1)
- `go vet ./internal/acp/ ./internal/acpserve/` — clean

## Decisions Made

- The set REQUEST's option-key field is `configId` (v1 `SetSessionConfigOptionRequest`) while the advertisement option key is `id` — both pinned against the fetched schema; the plan prose's "optionId" normalized to wire truth.
- The acp-side `ConfigSurface` froze as `Options / Set(sessionID, optionID, value) / ApplyBlobDefaults`; `acp.Server.NotifyConfigOptions` owns the config_option_update frame (foreground lane, Barrier-accounted). The sessionless initialize-blob update rides an empty sessionId — the connection has no view to re-render, the set still ships.
- D-10's explicitness boundary drawn at the LAYER FILES: only files are operator config the blob never overrides; the embedded floor is not — so the blob beats the floor in-memory, and a re-push of a blob-derived effective value is an idempotent no-op rather than a promotion into persisted config.
- Model writes go through the CURRENT tier's binding (`tiers.<tier>.model`), tier writes through `session_tier` — editor writes are simply another writer into the layers (D-12).
- The cross-provider live-apply guard lives in the surface (it owns config access); `runner.ApplyTurnModel` stays a pure model application — same loud-degrade contract as `resolveSubagentModel`.

## Deviations from Plan

**1. [Plan-structure note] Task 2's file list included internal/acp/handlers.go; no edit was needed**
The Task 1 frozen surface contract held against the real implementation — the handler needed zero changes when the production surface landed. Same file set otherwise; nothing was dropped.

**2. [Plan-structure note] config_option_update emission shape**
The plan left the emission vehicle executor-frozen. It landed as `Server.NotifyConfigOptions` (frame builder + foreground-lane route) called by the surface's notify hook, wired in Run — the SetEmitter precedent. The plan's "apply-as-landed" semantics for the sessionless blob case (empty sessionId) are documented in the code.

**Total deviations:** 0 auto-fixed bugs; 2 structural notes. **Impact:** low — both within the plan's own architecture and file set.

## Issues Encountered

- The cross-provider tier-switch serve test initially wrote a YAML document with a duplicate top-level `providers:` key (config-load failure surfaced loudly by the loader, exactly as designed); restructured the fixture into a single document. Test-side only.

## Authentication Gates

None.

## Known Stubs

None. The pending options (`permissions.mode`, `compaction-threshold`) advertise-and-log by DESIGN (D-05's apply-as-landed rule) — Phase 17/19 register real handlers behind the locked shape; nothing is fabricated in the meantime.

## Next Phase Readiness

- Ready for 16-06 (simulator smoke): the whole ACP-08 surface is drivable over the wire — advertisement with effective values, typed validated writes, out-of-band updates, and live model switching proven end-to-end in-repo; the simulator story and live-editor confirmation close ACP-08's criterion-4 leg.
- ACP-08 is NOT marked complete in REQUIREMENTS.md: the shared-ID gate (#2388) blocks it while sibling plan 16-06 (also declaring ACP-08) lacks a summary — the last declaring plan flips it.
- Phase 17/19 register pending handlers behind `isPendingOption`; Phase 18's load/resume reuse `configOptionsFor` unchanged; Phase 20's /model tops the precedence chain this surface already reads.

---
*Phase: 16-acp-wire-foundation*
*Completed: 2026-08-27*

## Self-Check: PASSED

- All 5 created files + SUMMARY exist on disk ✓
- All 7 commits present in history: 6c899cf, 9d53e7a, 007b6b1, eab5136, d3a5aed, a34d9e1, e06b0ab ✓
- TDD gate order verified: test → feat ×3 ✓
- All task acceptance criteria re-run and passing; plan-level verification (`go test -race` over the four packages, `mise ci` exit 0) green ✓
- ACP-08 NOT marked complete in REQUIREMENTS.md — shared-ID gate (#2388): sibling plan 16-06 also declares it and has no summary yet ✓
