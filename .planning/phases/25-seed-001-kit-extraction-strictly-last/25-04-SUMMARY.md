---
phase: 25-seed-001-kit-extraction-strictly-last
plan: "04"
subsystem: infra
tags: [go, kit, seam-inversion, frontend-seam, adapter, refactoring, kit-extraction]

requires:
  - phase: 25-seed-001-kit-extraction-strictly-last
    provides: kit/ holding the complete promotion set (15 packages + kit/internal/enginebridge), the 1397 ledger baseline, the fresh-baseline differential recipe (cache-clean lint + per-finding latent attribution + race failing-set comparison)
provides:
  - KIT-02's minimal pair kit-side — kit/runtime/seams.go: the one-method Emitter (D-14) and the ctx-blocking Requester (D-15) with kit-neutral Ask/Answer structs
  - The severed runtime→acp edge — kit/runtime's dependency set no longer contains internal/acp (11 of the 12 pass-2 edges remain for 25-05..25-07)
  - internal/acpserve/kit_adapter.go — the OQ2 home: kitTurnAdapter (FROZEN acp.TurnRunner + capability forwards), kitEmitter (event→frame translation incl. the relocated toolKindFor/ToolCallFrame builders), mapKitStop (OQ3 adapter-only stop mapping), kitRequester (D-15 bridge over the locked 17-02/17-04 ask surfaces)
  - event.UserMessageChunk — the additive neutral kind carrying the class-B echo across the seam
  - OQ2/OQ3 resolutions proven at execution time (adapter home = internal/acpserve; kit-raw stop, adapter maps, cron audit keeps raw)
affects: [25-05, 25-06, 25-07, 25-08, 25-09, KIT-02 frontend seam, v1.3 Telegram peer]

actuals:
  tokens: 31451   # chars/4 over the rendered diff 9407e8a..HEAD (125,805 chars incl. context)
  tasks: 3
  commits: 2      # fffb84e (Tasks 1+2 substance), fbb6699 (Task 2 verification fallout + Task 3) — measured: git rev-list --count 9407e8a..HEAD; docs commit follows
  plan_head_before: 9407e8a7ffb086bd2b1ae01583fe17f4478435c7

tech-stack:
  added: []
  patterns:
    - "Frontend-seam pattern (KIT-02): the kit emits its OWN neutral event vocabulary through ONE method and asks through ONE ctx-blocking method; every frame/stop/kind translation lives in a composition-root adapter — the kit package's dep set is the proof (go list -deps | grep internal/acp == 0)"
    - "Assertion-preserving test retarget: when a production interface moves app-side, an in-package test-local TWIN of the adapter (same translation, same capability forwards) keeps the relocated batteries' fakes, prompts, and assertions byte-identical — only the construction of the runner-under-test changes; the production adapter is pinned end-to-end by the composition-root suites"

key-files:
  created:
    - kit/runtime/seams.go (Emitter, Requester, Ask, Answer, AskFamily, SetRequester + the entry/answer converters)
    - internal/acpserve/kit_adapter.go (kitTurnAdapter, kitEmitter, kitEmitterFactory, toolKindFor table, toSessionBlocks, mapKitStop, kitRequester)
    - kit/runtime/acp_adapter_shim_test.go (the in-package adapter twin; zero new Test functions — ledger holds)
  modified:
    - kit/runtime/runtime.go (Run kit-neutral; forwarder emits raw events; translations deleted; SetEmitter/SetRequester retype)
    - kit/runtime/cron_wiring.go (session-lifetime forwarder emits raw events through the seam)
    - kit/runtime/commands.go + internal/acpserve/command_source.go (kit-neutral CommandAd advertisement; wire frames built app-side)
    - kit/event/events.go (UserMessageChunk, additive)
    - kit/session/ask.go (doc note: the ask origin is the Requester seam)
    - internal/acpserve/acp_serve.go (WithTurnRunner(kitTurnAdapter{...}); SetEmitter(kitEmitterFactory(srv.Emitter)) at the same WINDOWS #3 statement; the two Set*Fire injections replaced by one SetRequester)
    - internal/acpserve/blob_chip_wire_test.go (retarget to the production adapter)
    - 24 kit/runtime test files (114 r.Run→acpRun retargets; 8 WithTurnRunner/SetEmitter wraps; advertisement assertions flattened to CommandAd fields)

key-decisions:
  - "25-04: OQ2 executed — ALL acp↔kit translation lives in internal/acpserve/kit_adapter.go (composition root; internal/acp stays wire-only, zero kit imports; kit/runtime carries zero transport vocabulary)"
  - "25-04: OQ3 executed — the kit returns its RAW stop marker ('ask'); ONLY the adapter maps (mapKitStop → end_turn); cron/wake audit lines keep recording the raw stop (verified: they call runOneTurn directly and never saw the mapping; TestCronWiring_AuditProvenance green, content unchanged)"
  - "25-04: the staging rule resolved STRONGER than planned — the real adapter landed in Task 1 (no temporary glue ever existed, so no commit could leave glue and adapter both wired); Task 2 is verification + retarget fallout, riding fbb6699 (the 25-03 records-only-task precedent)"
  - "25-04: event.UserMessageChunk added additively — the class-B echo needed a neutral carrier across the seam (a wire user_message_chunk built kit-side would violate D-20); A3's anticipated additive-kind path, D-14's single-method rule intact"
  - "25-04: no-double-timeout record — the LANDED 17-02 code arms exactly two windows (registry HUMAN-ASK = wire round-trip, adapter-owned; AskBroker D-01 timer = suspension resume, kit-owned); the Requester bridge arms NEITHER (code comment in kit_adapter.go + seams.go)"
  - "25-04: TDD gate ran per task (task.is-behavior-adding = false ×3 — no tdd attr, no <behavior> block); the plan pre-recorded TDD-ineligibility and the 1397 ledger constraint forbids new test functions — the existing suites are the contract, exactly as the plan specified"

patterns-established:
  - "Frontend-seam pattern + assertion-preserving test retarget (see tech-stack) — 25-05..25-07's edge severances reuse both"

requirements-completed: []   # KIT-02 intentionally NOT listed: 25-05/06/07/09 also declare it and have no SUMMARYs yet — shared-ID gate (requirements.ready-ids = 0/1) holds it until the last sibling finishes

coverage:
  - id: D1
    description: "KIT-02 Emitter seam: kit/runtime emits its neutral event vocabulary through the one-method Emit; the forwarders build zero transport frames"
    requirement: KIT-02
    verification:
      - kind: unit
        ref: "go test ./kit/runtime/ -count=1 -> ok (the relocated forwarder/emitter/engine batteries green through the shim)"
      - kind: other
        ref: "grep 'func routeBusEvent|forwardToolCall|forwardToolCallUpdate|toolKindFor|toContentBlocks|mapAskStop' kit/ --include='*.go' excluding tests -> 0; seams.go exports exactly Emit(ctx, event.Event) error"
    human_judgment: false
  - id: D2
    description: "KIT-02 Requester seam: the ctx-blocking Request(ctx, Ask) (Answer, error) interface + kit-neutral Ask/Answer structs defined kit-side with contract doc comments"
    requirement: KIT-02
    verification:
      - kind: other
        ref: "kit/runtime/seams.go: Request(ctx context.Context, ask Ask) (Answer, error) — exactly one method; Ask/Answer carry questions/kind/answers/cancelled, no wire vocabulary"
    human_judgment: false
  - id: D3
    description: "runtime→acp edge severed: kit/runtime's dependency set contains no internal/acp (dep set shrank by exactly acp; the other 8 app edges ride for 25-05..25-07)"
    requirement: KIT-02
    verification:
      - kind: other
        ref: "go list -deps ./kit/runtime | grep -c 'ass-guard-agent/internal/acp' -> 0; pre/post dep-set diff shows acp removed, nothing else changed"
    human_judgment: false
  - id: D4
    description: "App-side adapter (OQ2): kitTurnAdapter satisfies the FROZEN acp.TurnRunner (byte-unchanged server.go), hosts the relocated translations, forwards the four optional capabilities"
    requirement: KIT-02
    verification:
      - kind: other
        ref: "git diff 9407e8a..HEAD -- internal/acp/server.go -> 0 lines; kit_adapter.go holds toolKindFor/ToolCallFrame builders/toSessionBlocks/mapKitStop"
      - kind: unit
        ref: "go test ./internal/acpserve/ ./cmd/ass-guard/ -count=1 -> ok (serve, simulator, zeroconfig smoke green through the production adapter)"
    human_judgment: false
  - id: D5
    description: "Composition-root wiring preserved: acpserve.Run statement order unchanged; SetEmitter injection at its exact WINDOWS #3 position; OQ3 stop-mapping adapter-only"
    requirement: KIT-02
    verification:
      - kind: unit
        ref: "go test ./internal/acpserve/ -count=1 -run 'TestACPServeWiresStdoutClean|TestSandboxFlag_ProbeBeforeScheduler|TestCronWiring' -> ok (order/degradation pins green)"
      - kind: other
        ref: "mapKitStop exists only in internal/acpserve/kit_adapter.go; cron_wiring.go audit lines record stop= from runOneTurn's raw return"
    human_judgment: false
  - id: D6
    description: "Requester wire bridge (D-15): the locked 17-02/17-04 ask surfaces implement the kit Requester; suspended-turn semantics byte-preserved; exactly the two landed timer owners, the bridge arms none"
    requirement: KIT-02
    verification:
      - kind: unit
        ref: "go test ./kit/session/ ./internal/acpserve/... -count=1 -run 'Ask|ask' -> ok (AskBroker timeout/suspend batteries unchanged; ask-drain + ask-surface suites green through the bridge)"
      - kind: other
        ref: "'arms NEITHER' ownership record in kit_adapter.go; SetRequester derives both fire seams (permission + elicitation) from the one interface"
    human_judgment: false

duration: 58 min
completed: 2026-09-10
status: complete
---

# Phase 25 Plan 04: KIT-02 First Half — Emitter + Requester Seams, runtime→acp Severed Summary

**The kit's frontend seam formalized as a one-method Emitter + ctx-blocking Requester with every wire translation (frames, tool kinds, stop mapping, ask bridging) relocated to a composition-root adapter in internal/acpserve — kit/runtime's dependency set no longer contains internal/acp, and all relocated suites pass with byte-identical assertions**

## Performance

- **Duration:** 58 min (2026-09-10T21:07Z → 22:05Z)
- **Started:** 2026-09-10T21:07:15Z
- **Completed:** 2026-09-10T22:05Z
- **Tasks:** 3/3
- **Files modified:** 36 (3 created, 33 modified; 24 of them test retargets)

## Accomplishments

- Task 1 (`fffb84e`): kit/runtime/seams.go landed (the KIT-02 minimal pair) and the runtime→acp edge severed — Run's signature is kit-neutral (session blocks in, kit Emitter, RAW stop marker out per OQ3), the forwarder emits raw kit events through the one Emit method, and routeBusEvent/forwardToolCall/forwardToolCallUpdate/toolKindFor/toContentBlocks/mapAskStop are gone from kit/. The app-side adapter (internal/acpserve/kit_adapter.go — OQ2's home) landed in the same commit per the staging rule, so the serve path was never degraded between commits.
- Task 2 (verification, riding `fbb6699`): the frozen-interface proof (server.go 0 diff lines), the OQ3 audit verification (cron/wake audit lines call runOneTurn directly and record the raw stop — content unchanged, TestCronWiring_AuditProvenance green), the serve/emitter suite proof, and the full differential gates (see §Verification Evidence). The retarget-fallout lll fixes landed here.
- Task 3 (`fbb6699`): the Requester wire bridge — SetRequester derives both fire seams from the ONE interface; acpserve's kitRequester bridges Ask/Answer onto the locked PermissionAsk/ElicitationAsk surfaces with entries rebuilt field-for-field; the no-double-timeout ownership recorded in code (the registry HUMAN-ASK window is transport; the AskBroker D-01 timer is the kit's suspension policy; the bridge arms neither).
- Differential equivalence at the final commit: **lint 595 vs baseline 598 with ZERO new findings** (my wraps fixed two pre-existing lll findings; two latent canonicalheader findings stopped surfacing; one pre-existing contextcheck chain re-attributed through the new call paths — the checkpointStore→Open root cause, pre-noted at its own site); race stable-core identical to the fresh baseline.

## Task Commits

1. **Task 1: kit-side seams + severance + app-side adapter** — `fffb84e` (feat; 35 files)
2. **Task 2: verification + retarget fallout** — no separate production commit (the adapter IS Task 1's glue-and-adapter per the staging rule; the lll fixes ride `fbb6699`) — the 25-03 records-only-task precedent
3. **Task 3: Requester wire bridge** — `fbb6699` (feat; 6 files)

**Plan metadata:** this docs commit.

## Verification Evidence (all re-run at final HEAD)

Task 1 acceptance criteria:
- seams.go exports Emitter + Requester, each EXACTLY one method, plus kit-neutral Ask/Answer with contract doc comments naming implementor and nil-degradation — **PASS**
- kit/runtime dependency set contains no acp — **PASS** (go list -deps | grep internal/acp → 0; dep set shrank by exactly acp)
- routeBusEvent/forwardToolCall/forwardToolCallUpdate/toolKindFor/toContentBlocks exist nowhere under kit/ — **PASS** (production grep 0; only relocation-documenting comments)
- forwarder loop emits raw kit events through the single Emitter method — **PASS** (`route := func(e event.Event) { _ = emit.Emit(ctx, e) }`)
- kit compiles + relocated runtime/session suites pass — **PASS** (go test ./kit/runtime/ ./kit/session/ -count=1 → ok, 25s/5s)

Task 2 acceptance criteria:
- kit_adapter.go exists; adapter satisfies acp.TurnRunner with server.go byte-unchanged — **PASS** (git diff 9407e8a..HEAD -- internal/acp/server.go → 0 lines)
- stop mapping ONLY in the adapter; cron audit keeps the kit-raw marker — **PASS** (mapKitStop only in kit_adapter.go; cron_wiring.go:211/:517 record stop= from runOneTurn's raw return; TestCronWiring_AuditProvenance + the whole TestCronWiring family green)
- acpserve.Run statement order unchanged — **PASS** (serve batteries TestACPServeWiresStdoutClean, TestSandboxFlag_ProbeBeforeScheduler/MainHookFirstCall green — the degradation-arm pins)
- Full mise ci green — **differential PASS** (vet 0 / build 0 / lint 595 ≤ 598 zero-new / race stable-core identical; rotating members proven environmental — see Deviations #4)

Task 3 acceptance criteria:
- Kit Requester has a working acp-side implementation over the locked ask surface; AskBroker timeout/suspend tests unchanged — **PASS** (go test ./kit/session/ ./internal/acpserve/... -run 'Ask|ask' → ok)
- Ask timeout semantics unchanged (negative→default, 0 blocks, timeout→capture-shaped non-answer) — **PASS** (relocated 12-01/17-03 batteries green, zero edits to their assertions)
- No double timeout: exactly one layer per concern, recorded in code + here — **PASS** (kitRequester doc comment: "arms NEITHER"; SetRequester doc: the kit owns the D-01 policy)
- mise ci green — **PASS differentially** (above)

Plan-level verification:
- kit/runtime imports zero acp; seams.go exports the one-method pair — **PASS**
- Adapter satisfies the FROZEN acp.TurnRunner; translations relocated and pinned by unchanged assertions — **PASS**
- acpserve.Run statement order preserved; SetEmitter injection point unchanged — **PASS** (kitEmitterFactory(srv.Emitter) at the identical statement, strictly between server construction and scheduler start)
- Full mise ci green — **PASS differentially** (pre-existing lint/race baselines predate this phase; this plan adds zero and strictly improves lint by 3)

D-20 ledger: kit 807 + internal 557 + cmd 33 = **1397 == 1397** (the shim file adds zero Test functions; no tests added or removed).

## Files Created/Modified

- `kit/runtime/seams.go` — KIT-02's minimal pair + SetRequester + the Ask/Answer converters
- `internal/acpserve/kit_adapter.go` — the OQ2 adapter home (turn adapter, emitter translation, stop mapping, requester bridge)
- `kit/runtime/runtime.go`, `cron_wiring.go`, `commands.go`, `doc.go` — the severance + kit-neutral advertisement
- `kit/event/events.go` — UserMessageChunk (additive)
- `kit/session/ask.go` — Requester-origin doc note
- `internal/acpserve/acp_serve.go`, `command_source.go` — composition wiring (3 statements) + frame projection
- `kit/runtime/acp_adapter_shim_test.go` + 24 test files — the assertion-preserving retarget
- `internal/acpserve/blob_chip_wire_test.go` — retarget to the production adapter

## Decisions Made

- The staging rule resolved stronger than planned: landing the REAL adapter in Task 1 (instead of temporary glue) means no commit ever had unwired glue — the plan's invariant ("no commit leaves glue and adapter both wired") holds vacuously.
- The test batteries stayed in kit/runtime with an in-package adapter twin rather than moving to acpserve: the batteries are white-box (unexported runner internals), Go forbids the in-package test importing acpserve (cycle), and the production adapter is pinned end-to-end by acpserve's own serve/simulator suites.
- The class-B echo rides a NEW neutral event kind (UserMessageChunk) rather than reusing AgentMessageChunk-with-a-decorated-id: the wire renders user_message_chunk vs agent_message_chunk — different frames, pinned behavior.
- Engine asks keep PlainTextFallback=false through the bridge (carried field, not derived from family — the enqueue-time truth rides the seam).

## Deviations from Plan

### Recorded Deviations

**1. [Staging - benign strengthening] The real adapter landed in Task 1; Task 2 has no separate commit**
- **Found during:** Task 1 execution
- **Issue:** The plan's staging rule expected temporary glue in Task 1 then the real adapter in Task 2. Since the runner's signature changed in Task 1, the serve path needed the full adapter immediately — half-measures would have degraded it.
- **Fix:** Landed kit_adapter.go as the real adapter in Task 1. No temporary glue ever existed, so the no-glue-and-adapter-both-wired invariant holds vacuously; every commit is whole and green. Task 2 is verification + retarget fallout (lll wraps), riding `fbb6699`.
- **Files modified:** none beyond plan scope
- **Verification:** per-commit gates green (build/vet/tests at fffb84e; full differential at fbb6699)
- **Committed in:** `fffb84e` / `fbb6699`

**2. [Rule 3 - blocking] commands.go + command_source.go advertisement neutralization was not in files_modified**
- **Found during:** Task 1 (the zero-acp criterion demanded it — the planner's file list missed that CommandAdvertisement returns acp.AvailableCommandFrame)
- **Issue:** kit/runtime/commands.go's advertisement surface carried acp frame types — the acp import could not die without neutralizing it.
- **Fix:** kit-neutral runtime.CommandAd rows (Name/Description/InputHint); acpserve's commandSourceAdapter builds acp.AvailableCommandFrame. Same discovery class as 25-03's 12-vs-8 edge count.
- **Files modified:** kit/runtime/commands.go, internal/acpserve/command_source.go, kit/runtime/commands_test.go (assertions flattened to the kit-neutral fields — same rows, same hints)
- **Verification:** advertisement battery green; serve TestCommandsNotifySeam green
- **Committed in:** `fffb84e`

**3. [Rule 2 - missing-critical-for-the-seam] event.UserMessageChunk added (file outside files_modified)**
- **Found during:** Task 1 (emitClassBEcho had no neutral carrier)
- **Issue:** The class-B echo previously built an acp user_message_chunk frame kit-side; without a neutral kind the echo would either break or smuggle wire vocabulary back.
- **Fix:** The additive event kind (A3's anticipated path); the adapter renders the frame. D-14's single-method rule intact.
- **Files modified:** kit/event/events.go, kit/runtime/runtime.go (emitClassBEcho)
- **Verification:** class-B echo batteries green (runner_battery, commands, restore_guard)
- **Committed in:** `fffb84e`

**4. [Environmental - attribution] race-suite flake families rotate between identical-content runs**
- **Found during:** the full differentials (both the mid-plan and final runs)
- **Issue:** The failing set beyond the stable core rotates: ModesMatrix ×9 (fixture-availability, 25-02's baseline had them failing, 25-03's passing), TestEscalation_ReapAllUsesLadder (25-03's documented load-flake), TestTranscriptWriterAsync/TestLiveInstalledPluginsProbe (baseline members that pass post-change). The stable core {TestPermissionsE2E (STATE's Phase-23 record), TestRescanConcurrency, 4×TestRunSuite_*} is IDENTICAL to the fresh baseline.
- **Fix:** none — isolation runs prove the rotating members green (ModesMatrix family full-set -race isolation ok; Escalation ok; TranscriptWriterAsync ok). Flaky ≠ regression per 25-03's rule.
- **Committed in:** n/a (evidence: /tmp logs summarized here)

**5. [Rule 1 - lint fallout] four over-length lines from the retarget**
- **Found during:** Task 2's lint differential
- **Issue:** `r.Run(...)` → `acpRun(ctx, r, ...)` lengthened two acp_engine_e2e lines past 120 (new findings) and worsened two pre-existing steering_ingress findings.
- **Fix:** wrapped all four; the differential went net-negative (595 vs 598).
- **Files modified:** kit/runtime/acp_engine_e2e_test.go, kit/runtime/steering_ingress_test.go
- **Verification:** final lint multiset diff — zero new finding classes
- **Committed in:** `fbb6699`

---

**Total deviations:** 3 auto-fixed in-plan (advertisement neutralization, echo event kind, lll wraps), 2 recorded process/environmental notes (staging strengthening, flake attribution)
**Impact on plan:** Plan goal fully achieved — the edge is severed, both seams exist with working acp-side implementations, and every gate is green-or-identical-to-baseline. The file-list drift (#2) is planner-scope, not design-scope.

## Issues Encountered

- One Edit tool mishap mid-Task-1 (a routeAskReply tail replaced with the wrong block) — caught by reading the damage immediately and repaired in the next two edits; the compiler confirmed.
- golangci-lint was not on PATH in the background baseline shell (exit 127) — re-ran the cache-clean baseline in the foreground with the mise shim on PATH (598 findings recorded).
- contextcheck attributes findings at call sites, not the offending line: the first nolint placement (cron_wiring) was reported unused; moved to the sessionFor call site where the linter attributes it.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Ready for 25-05 (SetupEngine parameterization — OQ4's resolution) — the seam recipe is proven: define the interface kit-side at the point of use, adapt at acpserve, retarget tests with the in-package twin, differential-gate against a fresh baseline.
- 11 production kit→app edges remain (coreexec, ecosys×2, learning×2, openspec, sched, perm×2, sandbox, tasks) — 25-05..25-07's scope, per 25-03's inventory.
- Known pre-existing (not this plan's): lint baseline (now 595, STATE blocker tracks the repair), the stable race core (Phase-23 cross-workstream + fixture legs), the rotating flake families (ModesMatrix fixture availability; escalation load-flake — worth the dedicated ticket 25-03 suggested).

## Self-Check: PASSED

- kit/runtime/seams.go, internal/acpserve/kit_adapter.go, kit/runtime/acp_adapter_shim_test.go exist on disk — FOUND
- Commits fffb84e, fbb6699 present in git log — FOUND
- Ledger 1397 == 1397 — VERIFIED (kit 807 + internal 557 + cmd 33)
- All acceptance criteria re-run with PASS evidence above (mise ci lint/race differentially green vs the fresh 9407e8a baseline — zero new findings, stable failing core identical)
