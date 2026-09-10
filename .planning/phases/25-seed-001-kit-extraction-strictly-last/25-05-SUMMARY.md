---
phase: 25-seed-001-kit-extraction-strictly-last
plan: "05"
subsystem: infra
tags: [go, kit, seam-inversion, scheduler-port, learning-port, setupengine-parameterization, kit-extraction, refactoring]

requires:
  - phase: 25-seed-001-kit-extraction-strictly-last
    provides: kit/ holding the complete promotion set + the severed runtime→acp edge (25-04's adapter discipline), the 1397 ledger baseline, the fresh-baseline differential recipe
provides:
  - The D-16 Scheduler port (kit/runtime/scheduler.go — exactly Now/Due/ClaimForFire/CatchUp) + the promoted Automation/FireEvent value structs (kit/schedule, json tags byte-identical) with internal/sched aliasing them — the runner core's schedule surface is kit-owned, the behavioral store stays app-side and satisfies the port structurally at the unchanged SetSchedule call site
  - The D-12 LearnedStore port (kit/internal/enginebridge — one-method (string,bool) Lookup) + the runtime.Learned port (Lookup + RecordCandidate + EntryCount) — enginebridge→learning AND runtime→learning both severed (go list -deps: 0 through kit/runtime)
  - SetupEngine(setup EngineSetup) per OQ4 — the openspec/learning hosting moved app-side (internal/acpserve/kit_setup.go loadEngineSetup + kitLearned adapter), hookdag.DefaultHooks stays kit self-config (recorded sub-decision)
  - The openspec residual record: kit/runtime's remaining openspec import is EXACTLY LoadCommandRegistry's mutability read (direct == transitive == 1), handed to 25-06's CommandCatalog.Mutability; the sched transitive residual rides internal/coreexec's cron quartet until 25-07's SessionToolkit
affects: [25-06, 25-07, 25-08, 25-09, KIT-02 seam family, D-19 gate scope]

actuals:
  tokens: 11199   # chars/4 over the rendered diff 44c65d2..HEAD
  tasks: 3
  commits: 3      # b0ca0b8, dbd4c11, 1aba451 — measured: git rev-list --count 44c65d2..HEAD; docs commit follows
  plan_head_before: 44c65d25532b5ab52ad2e179e079c372be71d7c1

tech-stack:
  added: []
  patterns:
    - "Cycle-safe value-struct promotion: when a promoted struct's aliasing app package is transitively imported by the kit package the plan named as host (sched→kit/runtime→coreexec→sched), the pure-data structs sit in a NEW leaf kit package (kit/schedule) both sides can import; the port stays at its consumer (kit/runtime) — D-05 per-case promotion honored, D-16's four-method letter honored"
    - "Optional-capability store extraction (the acp.SessionCloser pattern): one injected store value satisfies TWO structural views — the kit's four-method Scheduler port (the core's surface) and coreexec.CronStore (the interactive quartet's CRUD); the per-session extraction (cronStoreOrNil) preserves the nil → structured-no-store degrade with zero coreexec behavior change"
    - "Parameterized-explicit-step startup (OQ4/D-11/D-17): the app loads ecosystem values in a helper immediately before the kit's SetupEngine step — same order, same degradation arms; the kit consumes only kit-typed inputs"

key-files:
  created:
    - kit/schedule/schedule.go (Automation + FireEvent + ScheduleDisplay — promoted verbatim, json tags byte-identical; the cycle-safe leaf home)
    - kit/runtime/scheduler.go (the four-method Scheduler port + cronStoreOrNil optional-capability extraction)
    - internal/acpserve/kit_setup.go (loadEngineSetup: openspec config + RegisterTools + FromConfig pattern table + learning.Open → kitLearned adapter)
    - kit/runtime/engine_setup_test.go (the in-package twin loader + testLearned adapter; zero new Test functions)
  modified:
    - kit/runtime/runtime.go (schedule Scheduler field, SetSchedule(Scheduler), SetupEngine(EngineSetup), Learned port, EngineSetup struct)
    - kit/runtime/cron_wiring.go (kit/schedule types; the four call sites unchanged against the port)
    - kit/runtime/commands.go (/memory consumes EntryCount through the port)
    - kit/internal/enginebridge/enginebridge.go (LearnedStore port; BridgeConfig.Learned + dispatcher retyped; learning import gone)
    - internal/sched/sched.go (type aliases to the kit structs; store + parser untouched)
    - internal/coreexec/cron.go + planmode.go (CronStore consumer-side interface; InteractiveConfig.Schedule retyped)
    - internal/acpserve/acp_serve.go (the engine arm: helper immediately before SetupEngine; everything else untouched)
    - 14 kit/runtime test files (19 SetupEngine call sites retargeted to the twin; ask_wiring + e2e Criterion4 inject via testLearned; 2 cron sites use the store handle)

key-decisions:
  - "25-05: cycle-forced promotion home — the plan's kit/runtime home for Automation/FireEvent is impossible: internal/sched aliasing kit/runtime cycles (kit/runtime→coreexec→sched→kit/runtime). The structs promote to the kit/schedule leaf (sched aliases it, no cycle); the Scheduler PORT stays in kit/runtime per D-16's consumer-side letter. The must_haves ('promote to kit/', 'exactly four methods') hold; the artifact line's file path is the only drift"
  - "25-05: OQ4 sub-decision recorded — hookdag.DefaultHooks stays INSIDE SetupEngine (kit-owned machinery self-loading its embedded defaults, the same legitimacy as kit/modelrouting's embedded config); everything ecosystem-hosted (openspec config, tool registration, pattern table, learning) is an EngineSetup input"
  - "25-05: the plan's Task-2 premise 'the adapter is constructed in acpserve.Run where the store opens today' was factually wrong (the store opened inside SetupEngine, kit-side) and kit/internal/enginebridge is kit-private (unimportable from acpserve) — the adapter landed with Task 3's loader where it is constructed; Task 2's between-commits liveness invariant held via the temporary nil-safe learnedPortAdapter (deleted in Task 3's same-commit switchover, as planned)"
  - "25-05: runtime.Learned carries THREE methods, not the plan's one — execution-time inventory found RecordCandidate (enqueueEngineAsk, 17-04 A9) and List (builtinMemory's /memory count) beyond the Lookup the plan enumerated; EntryCount is the kit-neutral count shape (Entry never crosses)"
  - "25-05: residuals recorded, not fixed (scope discipline) — openspec remains in kit/runtime's dep set EXACTLY as LoadCommandRegistry's mutability read (25-06's CommandCatalog.Mutability consumes it); internal/sched remains in kit/runtime's TRANSITIVE set via coreexec's cron quartet (25-07's SessionToolkit moves that registration app-side)"

patterns-established:
  - "Cycle-safe promotion + optional-capability store extraction + twin-loader test retarget (see tech-stack) — 25-06/25-07's remaining edges reuse all three"

requirements-completed: []   # KIT-01/KIT-02/KIT-03 all still declared by 25-06..25-09 (no SUMMARYs yet) — the shared-ID gate (requirements.ready-ids) holds them until the last sibling finishes, the 25-03/25-04 precedent

coverage:
  - id: D1
    description: "D-16 Scheduler port: the four-method interface in kit/runtime; cron_wiring's four call sites compile unchanged against it; nil = the documented degraded state"
    requirement: KIT-01
    verification:
      - kind: unit
        ref: "go test ./kit/runtime/ ./internal/sched/ -count=1 -> ok (cron batteries green, unchanged assertions; 2 sites retargeted to the store handle for SetNow)"
        status: pass
      - kind: other
        ref: "kit/runtime/scheduler.go exports exactly Now/Due/ClaimForFire/CatchUp (grep count 4); direct internal/sched import in kit/runtime = 0"
        status: pass
    human_judgment: false
  - id: D2
    description: "Automation/FireEvent promoted with byte-identical json tags; internal/sched compiles via aliases; persisted YAML/JSON round-trip untouched"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "diff of the promoted struct blocks vs git show HEAD~3:internal/sched/sched.go -> byte-identical (Automation incl. nolint comments, FireEvent); go test ./internal/sched/ -> ok (round-trip batteries unchanged)"
        status: pass
    human_judgment: false
  - id: D3
    description: "The store satisfies the port structurally at the UNCHANGED SetSchedule call site (no adapter); the interactive quartet's CRUD half rides coreexec.CronStore on the same value"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "acp_serve.go:494 runner.SetSchedule(scheduleStore) untouched (git diff 44c65d2..HEAD shows only the engine-arm change in acpserve); *sched.ScheduleStore satisfies both interfaces structurally (build-level proof); cronStoreOrNil preserves the nil no-store degrade"
        status: pass
      - kind: unit
        ref: "go test ./internal/coreexec/ -count=1 -> ok (cron quartet batteries over the CronStore interface, unchanged assertions)"
        status: pass
    human_judgment: false
  - id: D4
    description: "LearnedStore port (D-12): one-method (string,bool) Lookup in kit/internal/enginebridge; no app type in the signature; enginebridge→learning severed; ErrAskPending nil path preserved"
    requirement: KIT-02
    verification:
      - kind: other
        ref: "go list -deps ./kit/internal/enginebridge | grep -c 'ass-guard-agent/internal/learning$' -> 0"
        status: pass
      - kind: unit
        ref: "go test ./kit/internal/enginebridge/ ./kit/runtime/ -count=1 -> ok (Ask/ask batteries green through the port; Criterion4 + ask_wiring pinned via the testLearned twin)"
        status: pass
    human_judgment: false
  - id: D5
    description: "runtime→learning FULLY severed; SetupEngine parameterized per OQ4 with the kit-neutral EngineSetup (Catalog/PatternTable/Learned — no app type); learning open degradation identical (log-and-continue)"
    requirement: KIT-02
    verification:
      - kind: other
        ref: "go list -deps ./kit/runtime | grep -c 'ass-guard-agent/internal/learning$' -> 0 (direct AND transitive); EngineSetup fields are *toolcat.Catalog / engine.PatternTable / Learned — kit-defined only"
        status: pass
      - kind: unit
        ref: "go test ./internal/acpserve/ ./cmd/ass-guard/ -count=1 -> ok (serve/simulator/zeroconfig batteries green; engine-on/engine-off arms identical; helper adjacent to SetupEngine in the same arm, statement order otherwise untouched)"
        status: pass
    human_judgment: false
  - id: D6
    description: "openspec residual recorded precisely: kit/runtime's remaining openspec dependency is exactly LoadCommandRegistry's mutability read (direct == transitive == 1), handed to 25-06"
    requirement: KIT-02
    verification:
      - kind: other
        ref: "go list -f '{{join .Imports \",\"}}' ./kit/runtime | grep -c internal/openspec -> 1 (LoadCommandRegistry + the MutabilityMutating comparison in expandUserBlocks); go list -deps -> 1 (nothing else pulls it)"
        status: pass
    human_judgment: false
  - id: D7
    description: "D-20 equivalence: split gates green; lint/race differentials vs the fresh 44c65d2 baseline — zero new findings, identical race failing set; ledger 1397 == 1397"
    requirement: KIT-03
    verification:
      - kind: other
        ref: "cache-clean lint 598 vs baseline 596 — the +2 are the 25-03-documented latent canonicalheader pair (img_capability_test.go:245/343, a file this diff never touches, 0 diff lines); race failing set == baseline 7/7 by name (PermissionsE2E, RescanConcurrency, 4xRunSuite, LiveInstalledPluginsProbe); vet 0 / build 0; ledger kit 807 + internal 557 + cmd 33 = 1397"
        status: pass
    human_judgment: false

duration: 28 min
completed: 2026-09-10
status: complete
---

# Phase 25 Plan 05: Scheduler Port + Learning Ports + SetupEngine Parameterization Summary

**Three more kit→app edges severed — the runner core speaks scheduling through its own four-method Scheduler port (store satisfies it structurally at the unchanged call site), learning reaches the kit only through (string,bool) ports, and SetupEngine takes kit-neutral EngineSetup inputs with the openspec/learning hosting moved app-side — all batteries green with unchanged assertions and zero new lint/race findings vs the fresh baseline**

## Performance

- **Duration:** 28 min (2026-09-10T21:51Z → 22:19Z)
- **Started:** 2026-09-10T21:51:08Z
- **Completed:** 2026-09-10T22:19:00Z
- **Tasks:** 3/3
- **Files modified:** 27 (4 created, 23 modified; 14 of them test retargets)

## Accomplishments

- Task 1 (`b0ca0b8`): the D-16 Scheduler port landed — kit/runtime/scheduler.go with EXACTLY the four methods the runner core calls (Now/Due/ClaimForFire/CatchUp), the Automation/FireEvent value structs promoted with byte-identical json tags (proven by mechanical diff against the pre-move definitions), internal/sched aliasing them so its public API and persisted round-trips compile untouched, and the store satisfying the port structurally at the UNCHANGED SetSchedule call site. The plan's kit/runtime home for the structs was import-cycle-impossible (sched→kit/runtime→coreexec→sched); they promote to the kit/schedule leaf instead. The interactive cron quartet's CRUD half — a consumption site the plan missed — rides a new consumer-side coreexec.CronStore interface satisfied structurally by the SAME store value (the acp.SessionCloser optional-capability pattern).
- Task 2 (`dbd4c11`): the D-12 LearnedStore port — one method, (string, bool) Lookup, defined at the point of use in kit/internal/enginebridge; BridgeConfig.Learned retyped; enginebridge's dependency set no longer contains internal/learning. The between-commits liveness invariant held via a temporary nil-safe adapter at the BridgeConfig construction site (deleted in Task 3's same-commit switchover exactly as the plan staged).
- Task 3 (`1aba451`): SetupEngine(setup EngineSetup) per OQ4 — openspec config + tool registration (D-17: the app registers tools into the kit-owned catalog), the FromConfig pattern table, and learning.Open all moved to acpserve's loadEngineSetup helper (immediately before SetupEngine, same order, same degradation arms); hookdag.DefaultHooks stays kit self-config (recorded sub-decision). kit/runtime's learning dependency is FULLY gone (direct and transitive); the openspec residual is exactly the LoadCommandRegistry mutability read, recorded for 25-06.

## Task Commits

1. **Task 1: Scheduler port (D-16) — interface, value-struct promotion, structural satisfaction** — `b0ca0b8` (feat; 8 files)
2. **Task 2: LearnedStore port — one-method seam in enginebridge** — `dbd4c11` (feat; 2 files)
3. **Task 3: SetupEngine parameterization (OQ4)** — `1aba451` (feat; 20 files, 14 of them the mechanical SetupEngine/test retarget)

**Plan metadata:** this docs commit.

## Verification Evidence (all re-run at final HEAD 1aba451)

Task 1 acceptance criteria:
- scheduler.go exports the four-method port + two promoted structs with byte-identical json tags — **PASS** (method grep = 4; struct-block diff vs pre-move = byte-identical incl. nolint comments)
- kit/runtime's dependency set no longer contains internal/sched (edge severed) — **PASS at the DIRECT-import level** (0 direct); the TRANSITIVE set keeps internal/sched via internal/coreexec's cron quartet — recorded residual: coreexec legitimately owns the CRUD tools (app-side per KIT-03) until 25-07's SessionToolkit moves the per-session registration app-side; the plan's `go list -deps == 0` target is unsatisfiable before 25-07 (the planner missed the sessionFor→InteractiveConfig flow)
- internal/sched compiles via aliases; YAML round-trip tests unchanged — **PASS** (go test ./internal/sched/ ok, zero test edits)
- The store satisfies the port at the unchanged call site — **PASS** (no adapter: SetSchedule(scheduleStore) untouched; SUMMARY asserts none was needed)
- Cron batteries green with unchanged assertions — **PASS** (full kit/runtime suite ok; 2 sites retargeted to the returned store handle for SetNow — construction change only, the 25-04 discipline)

Task 2 acceptance criteria:
- LearnedStore exists with the (string,bool) signature; no app type — **PASS**
- enginebridge's dep set contains no internal/learning — **PASS** (go list -deps grep = 0); kit/runtime's learning import remained until Task 3 as planned (intermediate state live: the engine batteries ran green at dbd4c11 with the temp adapter)
- ErrAskPending nil-store path preserved and pinned — **PASS** (enginebridge suite + the ask batteries; the nil-check + early-return semantics untouched)
- The adapter lives in internal/acpserve/kit_setup.go — **PASS** (landed with Task 3's loader, which constructs it — see Deviations #3)

Task 3 acceptance criteria:
- SetupEngine's signature takes the kit-neutral inputs struct; no openspec-typed value in any kit signature — **PASS** (EngineSetup{Catalog *toolcat.Catalog, PatternTable engine.PatternTable, Learned Learned}; *OpenSpecPatternTable crosses only as the structural engine.PatternTable)
- kit/runtime's dep set: learning FULLY gone (0 direct, 0 transitive) — **PASS**; openspec residual recorded precisely — **PASS** (direct == transitive == 1, the LoadCommandRegistry mutability read → 25-06's CommandCatalog.Mutability, exactly the plan's anticipated residual)
- Engine-on/engine-off degradation arms identical — **PASS** (acpserve serve/simulator/zeroconfig batteries green; the helper failure arm logs the SAME "engine setup failed (continuing without engine)" line and skips SetupEngine)
- acpserve.Run statement order preserved with the helper adjacent to SetupEngine — **PASS** (git diff shows the only acpserve change is inside the engine-enabled arm; LoadCommandRegistry → [helper → SetupEngine] → SetRequester → sched.Open → SetSchedule → SetEmitter (WINDOWS #3) → StartScheduler)

Plan-level verification:
- Three edges severed (sched direct, learning ×2 full) with both residuals recorded precisely — **PASS** (see above)
- The four-method port + byte-identical promoted structs — **PASS**
- SetupEngine parameterized with the hookdag self-config sub-decision recorded — **PASS**
- mise ci green — **PASS differentially**: vet 0 / build 0 / cache-clean lint 598 vs the fresh 596 baseline with ZERO new findings (the +2 = the 25-03-documented latent canonicalheader pair on img_capability_test.go:245/343, a file with 0 diff lines this plan — latent resurfacing, not regression) / race failing set IDENTICAL to the fresh baseline 7/7 by name

D-20 ledger: kit 807 + internal 557 + cmd 33 = **1397 == 1397** (engine_setup_test.go adds zero Test functions — the twin-loader precedent).

TDD gate: `task.is-behavior-adding` = false × 3 (no tdd attr, no behavior block) — the plan's pre-recorded TDD-ineligibility held; the existing suites are the contract, the 25-04 precedent.

## Files Created/Modified

- `kit/schedule/schedule.go` — the promoted pure-data structs (byte-identical tags)
- `kit/runtime/scheduler.go` — the D-16 port + the optional-capability extraction
- `internal/acpserve/kit_setup.go` — loadEngineSetup + the kitLearned adapter
- `kit/runtime/engine_setup_test.go` — the twin loader + testLearned (zero Test functions)
- `kit/runtime/runtime.go`, `cron_wiring.go`, `commands.go` — the severance + parameterization
- `kit/internal/enginebridge/enginebridge.go` — the LearnedStore port
- `internal/sched/sched.go` — aliases; `internal/coreexec/cron.go`, `planmode.go` — CronStore
- `internal/acpserve/acp_serve.go` — the engine arm helper
- 14 kit/runtime test files — the mechanical retarget (19 SetupEngine sites + 2 cron store handles + 2 learning injections)

## Decisions Made

- The promoted structs' home is kit/schedule (new leaf), not the planned kit/runtime: the alias direction the plan specified is import-cycle-impossible (kit/runtime→coreexec→sched→kit/runtime). The port stayed in kit/runtime; the must_hives ("promote to kit/", "exactly four methods") hold intact.
- The interactive quartet's schedule surface became coreexec.CronStore (consumer-side interface over the alias types): one injected store value satisfies the four-method kit port AND the six-method CRUD view structurally; nil semantics byte-preserved (nil port → nil CronStore → structured no-store errors).
- runtime.Learned is a three-method port (Lookup + RecordCandidate + EntryCount): execution-time inventory found two consumption sites beyond the plan's one (the 17-04 A9 candidate write and the /memory entry count); EntryCount is the kit-neutral count shape — learning.Entry never crosses.
- hookdag.DefaultHooks stays inside SetupEngine (OQ4 sub-decision): kit-owned machinery self-loading embedded defaults mirrors kit/modelrouting's embedded config legitimacy.
- The two residuals stay UNFIXED (scope discipline): openspec-via-LoadCommandRegistry → 25-06, sched-via-coreexec → 25-07. Both successor plans already own them by name.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - blocking] kit/schedule leaf package created (files_modified drift)**
- **Found during:** Task 1 (the compile could not work as planned)
- **Issue:** The plan's alias design (internal/sched aliasing structs defined in kit/runtime) creates the import cycle sched→kit/runtime→coreexec→sched — Go rejects it at compile time. The planner did not account for coreexec's sched import.
- **Fix:** The pure-data structs promote to a NEW leaf kit package (kit/schedule) that both sched (alias) and kit/runtime (port signatures) import without cycles. The Scheduler port remains in kit/runtime/scheduler.go per D-16. Same discovery class as 25-04's advertisement neutralization.
- **Files modified:** kit/schedule/schedule.go (new), internal/sched/sched.go (aliases)
- **Verification:** byte-identity diff of the promoted blocks; sched suite green unchanged
- **Committed in:** `b0ca0b8`

**2. [Rule 3 - blocking] coreexec.CronStore interface (a consumption site the plan missed)**
- **Found during:** Task 1 (sessionFor would not compile after the retype)
- **Issue:** sessionFor passes r.schedule into coreexec.InteractiveConfig{Schedule: *sched.ScheduleStore} — the planner's read_first framed the schedule surface as only cron_wiring's four calls. After the retype the concrete-typed field cannot receive the port.
- **Fix:** Consumer-side CronStore interface in coreexec (the quartet's actual method set: Now/Get/List/Create/Save/Delete) + the cronStoreOrNil optional-capability extraction in kit/runtime. The same store value satisfies both views structurally; nil semantics identical; coreexec's own batteries run unchanged.
- **Files modified:** internal/coreexec/cron.go, internal/coreexec/planmode.go, kit/runtime/scheduler.go, kit/runtime/runtime.go (1 line at the sessionFor literal)
- **Verification:** coreexec + kit/runtime suites green; the no-store error batteries pass untouched
- **Committed in:** `b0ca0b8`

**3. [Rule 3 - blocking] kit_setup.go landed with Task 3, not Task 2**
- **Found during:** Task 2 (the plan's construction premise failed)
- **Issue:** The plan said the adapter is "constructed in acpserve.Run where the store opens today" — but the store opened inside SetupEngine (kit-side) until Task 3, and kit/internal/enginebridge is kit-PRIVATE (the Go internal rule forbids acpserve importing it for the var _ pin). An unused adapter type would also trip the unused linter.
- **Fix:** Task 2 landed the port + the temporary nil-safe adapter at the BridgeConfig site (learning lookups LIVE between commits — the plan's invariant); kit_setup.go landed with Task 3's loader, which constructs kitLearned immediately.
- **Files modified:** none beyond plan scope (sequencing only)
- **Verification:** per-commit builds + batteries green at dbd4c11 and 1aba451
- **Committed in:** `dbd4c11` / `1aba451`

**4. [Rule 2 - missing-critical-for-the-seam] runtime.Learned is three methods, not one**
- **Found during:** Task 3 (the compiler surfaced the sites the plan's inventory missed)
- **Issue:** The plan enumerated only Lookup; the runner also consumes RecordCandidate (enqueueEngineAsk's accepted-answer write, 17-04 A9) and List (builtinMemory's /memory entry count). A one-method port would have left runtime→learning unsevered.
- **Fix:** runtime.Learned = enginebridge.LearnedStore (the plan's one-method port, embedded) + RecordCandidate + EntryCount (kit-neutral count). The BridgeConfig assignment rides interface subsumption.
- **Files modified:** kit/runtime/runtime.go (Learned + EngineSetup), kit/runtime/commands.go (EntryCount), internal/acpserve/kit_setup.go, kit/runtime/engine_setup_test.go (testLearned)
- **Verification:** learning dep set 0 through kit/runtime; /memory + ask batteries green
- **Committed in:** `1aba451`

**5. [Rule 1 - lint fallout] new-file lint findings (lll/nlreturn/godoclint/wrapcheck)**
- **Found during:** per-task lint checks
- **Issue:** First drafts of scheduler.go/kit_setup.go/engine_setup_test.go plus one test comment tripped 8 findings (lll, nlreturn, godoclint package-comment, wrapcheck, ireturn).
- **Fix:** reformatted/restructured; final per-file lint: zero findings on every new file. The acp_serve.go cyclop finding is the pre-existing baseline member (17→18 complexity, same single finding).
- **Files modified:** the four new files + ask_wiring_test.go
- **Verification:** final full lint multiset — zero new findings vs baseline (the +2 count = the latent canonicalheader pair, untouched file)
- **Committed in:** `b0ca0b8` / `dbd4c11` / `1aba451`

---

**Total deviations:** 5 auto-fixed in-plan (3 blocking compile/premise issues, 1 missing-critical port surface, 1 lint fallout), 0 architectural stops
**Impact on plan:** Plan goal fully achieved — all three named edges severed (learning fully; sched at the direct level with the transitive residual mechanically unavoidable before 25-07), SetupEngine parameterized per OQ4, every gate green-or-identical-to-baseline. The two residuals are the plan's own anticipated handoffs, recorded for their named successors.

## Issues Encountered

- The first Task-3 commit missed 12 sed-modified test files (staged 7 of 19 retargeted files); caught by the post-commit status check and folded in via `git commit --amend` before any other work — the commit is whole.
- TestCompactionLive_MenuThresholdRoundTrip flaked in one full acpserve run; passes in isolation (the documented load-flake family — not this plan's; kit/ is not even in its dependency path).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Ready for 25-06 (CommandCatalog — the ecosys/openspec edges): the openspec residual is EXACTLY LoadCommandRegistry's mutability read + the MutabilityMutating comparison in expandUserBlocks; the cycle-safe-promotion and consumer-side-interface recipes are proven.
- Ready for 25-07 (SessionToolkit — coreexec): cronStoreOrNil + coreexec.CronStore is the seam it absorbs; severing kit/runtime→coreexec also kills the last transitive sched path (the D-19 gate's zero-app-deps bar for kit/runtime).
- Edge ledger after this plan: 8 of 12 pass-2 edges remain (coreexec, ecosys ×2, openspec-half, perm ×2, sandbox, tasks drift, session→perm per 25-03's inventory minus the 4 severed/absorbed here: sched-direct, learning ×2).
- Known pre-existing (not this plan's): the lint baseline (596/598 latent-stable, STATE blocker tracks the repair), the 7-member race failing set (identical to baseline), the rotating flake families (ModesMatrix fixtures, escalation load-flake, CompactionLive under full-suite load).

## Self-Check: PASSED

- kit/schedule/schedule.go, kit/runtime/scheduler.go, internal/acpserve/kit_setup.go, kit/runtime/engine_setup_test.go exist on disk — FOUND
- Commits b0ca0b8, dbd4c11, 1aba451 present in git log — FOUND
- Ledger 1397 == 1397 — VERIFIED (kit 807 + internal 557 + cmd 33)
- Dep severance re-verified at final HEAD: learning 0 through kit/runtime AND enginebridge; sched 0 direct (transitive 1 = the recorded coreexec residual); openspec 1 = the recorded LoadCommandRegistry residual
- All acceptance criteria re-run with PASS evidence above (the two residuals recorded per the plan's own residual clauses)

---
*Phase: 25-seed-001-kit-extraction-strictly-last*
*Completed: 2026-09-10*
