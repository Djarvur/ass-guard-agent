---
phase: 25-seed-001-kit-extraction-strictly-last
plan: "06"
subsystem: infra
tags: [go, kit, seam-inversion, command-catalog, session-types, kit-extraction, refactoring, d-17]

requires:
  - phase: 25-seed-001-kit-extraction-strictly-last
    provides: kit/ holding the complete promotion set with 4 of 12 pass-2 edges severed (25-04's acp adapter, 25-05's Scheduler/Learned/SetupEngine), the 1397 ledger baseline, the fresh-baseline differential recipe, the openspec residual record
provides:
  - kit/session/types.go — the kit-neutral session vocabulary (AgentDef mirror, the Hooks interface with Fire/PreToolUseVerdict/PostToolUse, HookOutcome, HookVerdict, RuleSet/RuleVerdict) — session→ecosys AND session→perm both severed (go list -deps ./kit/session: ZERO internal/* at any depth)
  - kit/runtime/catalog.go — the D-17 CommandCatalog seam (14 consumption-derived methods), the kit Command record with the promoted Expand substitution, the promoted ParseInvocation/ParseMentions pure parsers, and the emptyCatalog nil degradation
  - internal/acpserve/discover.go + catalog_adapter.go — the app-side discovery startup step (degradation arms verbatim) and the ONE app→kit mapping site; acpserve.Run's SetCatalog sits at LoadCommandRegistry's exact statement position
  - The edge-ledger state after this plan: kit/runtime's internal dep set is EXACTLY {coreexec, perm, sandbox, sched-via-coreexec, tasks} — ecosys and openspec gone direct AND transitive (the 25-05 openspec residual closed)
affects: [25-07, 25-08, 25-09, KIT-02 seam family, D-19 gate scope]

actuals:
  tokens: 33935   # chars/4 over the rendered diff 5a68cfc..HEAD (kit/ + internal/acpserve/)
  tasks: 2
  commits: 2      # 097711e, 9ac3aab — measured: git rev-list --count 5a68cfc..HEAD; docs commit follows
  plan_head_before: 5a68cfce3c58e2504339e170778ae89a4df88f53

tech-stack:
  added: []
  patterns:
    - "Catalog injection with promoted pure parsing (D-17): registry/ecosystem access goes through the injected CommandCatalog; PURE text parsers (ParseInvocation, ParseMentions, Expand substitution) promote kit-side instead of catalog-method'ing so invocation detection and expansion degrade identically with or without a catalog (the nil-degradation invariant: /undo routing and class-B detection never depend on discovery)"
    - "Consumption-derived interface at the consumer (25-PATTERNS Interface-at-consumer): every CommandCatalog method is a live consumption site — the planning-time 9-method enumeration grew to 14 at execution time because commands.go's chain machinery, rescan.go, mentions, and memory were equally runtime→ecosys edges the plan's runtime.go-only line inventory missed"
    - "Test-twin adapter for white-box batteries (the 25-05 engine_setup precedent at catalog scale): kit/runtime tests cannot import internal/acpserve (cycle), so catalog_twin_test.go mirrors the adapter's construction, degradation arms, and mappings; production kits stay app-import-free while the batteries keep real discovery over planted trees"
    - "Nil-safe emptyCatalog degradation: a never-installed catalog returns every method's documented empty value (expansion misses = plain text, listings no-op, hooks/agents unwired, builtins still resolve) — T-25-28's never-panic contract, the stub-exec family"

key-files:
  created:
    - kit/session/types.go (AgentDef, Hooks, HookOutcome, HookVerdict, RuleSet/RuleVerdict, the mcp__ canonicalization mirror; mirror-or-promote justified per type)
    - kit/runtime/catalog.go (the D-17 seam + Command/Expand + promoted parsers + emptyCatalog)
    - internal/acpserve/discover.go (loadCommandCatalog: the relocated discovery step, arms verbatim)
    - internal/acpserve/catalog_adapter.go (the ONE app→kit mapping site: agents, hooks, commands, MCP layers, mutability)
    - kit/runtime/catalog_twin_test.go (the white-box twin + plant helpers; zero new Test functions)
    - kit/session/adapt_twin_test.go (testHooks/testRuleSet twins for the session batteries)
  modified:
    - kit/session/{session,subagent,gate,ask}.go (the kit vocabulary retypes; gate.go's kit enums + canonicalToolName)
    - kit/runtime/runtime.go (catalog field + SetCatalog; sessionFor/listings/hooks/agents/SkillExecute/MCP-merge rewired; LoadCommandRegistry, registry fields, CommandRegistry/registrySnapshot, Task-1 temp mappings deleted)
    - kit/runtime/commands.go (chainEntry kit types; buildChain over []CatalogEntry with the agent-dispatch-hint kit stamp; resolveSlashCommand/skillBodyEmpty via catalog; /memory /mcp /doctor via catalog)
    - kit/runtime/rescan.go (rescanAndSwap rides catalog.Rediscover)
    - internal/acpserve/acp_serve.go (the one-statement SetCatalog swap at LoadCommandRegistry's position)
    - 20 kit/runtime test files (mechanical LoadCommandRegistry→SetCatalog(twin) retarget + fixture retypes)

key-decisions:
  - "25-06: ParseInvocation/ParseMentions/Expand PROMOTED kit-side as pure functions, not catalog methods — the plan's must_hive enumeration listed ParseInvocation as a catalog method, but the nil-catalog degradation invariant (builtins, /undo routing, and class-B detection must parse identically without discovery) forces pure functions; the catalog owns only registry/ecosystem state"
  - "25-06: the catalog's method set is 14, not the plan's 9 — commands.go's chain machinery (buildChain/installRegistry/resolveSlashCommand), rescan.go's Discover, the mention parser, memory files/injection, and /doctor's counts are equally runtime→ecosys consumption sites; the verify clause (zero ecosys in go list -deps) is absolute, so the seam covers them all (Rule 3, documented as Deviation 1)"
  - "25-06: session→perm severed INSIDE Task 1 (the 25-03-assigned residual): kit RuleSet interface + RuleVerdict enum + the inline mcp__ canonicalization mirror in kit/session/types.go; the composition-side kitRuleSet adapter lives in kit/runtime (which keeps its own perm edge until a later plan) and moves app-side when that edge severs"
  - "25-06: LookupCommand(kind, key) carries the chain-winner kind — the chain (kit) decides WHO won, the catalog (app) fetches the body; file bodies resolve from the live registry at resolve time (post-maybeFreshRescan the two are in sync — the 20-05 freshness discipline, unchanged for skills by design)"
  - "25-06: chainEntry's skill/file registry pointers DELETED — the chain holds only display metadata + the agent AgentDef; bodies fetch through the catalog at resolve time (the chain stays a pure view; class-A /init synthesizes a kit Command)"
  - "25-06: the agent dispatch hint ('(prompt for the agent)') is stamped kit-side in buildChain — it is the kit's SKLS-02 wire convention, not discovery data; catalog rows do not carry it"

requirements-completed: []   # KIT-01/02/03 are shared with 25-07..25-09 (no SUMMARYs yet) — the shared-ID gate (requirements.ready-ids) holds them until the last sibling finishes, the 25-03/25-04/25-05 precedent

coverage:
  - id: D1
    description: "kit/session holds zero ecosys vocabulary: SubagentTypes is map[AgentDef], Hooks is the kit interface (Fire + PreToolUseVerdict + PostToolUse), HookOutcome flows through injectHookContext; every Fire/hook call site compiles against the kit interface"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "go list -deps ./kit/session | grep -c 'ass-guard-agent/internal/ecosys$' -> 0 (grep exit 1); the full internal dep set of kit/session is EMPTY (zero internal/* at any depth)"
        status: pass
      - kind: unit
        ref: "go test ./kit/session/ ./kit/runtime/ -count=1 -> ok (5.3s / 34.1s; hooks_seam/subagent/subagent_types batteries green via the adapt_twin_test twins, assertion-preserving)"
        status: pass
    human_judgment: false
  - id: D2
    description: "The 25-03-assigned session→perm residual severed: GateDeps.Rules is the kit RuleSet view, the verdict enums are kit-defined, the MCP-name canonicalization is a kit-side mirror"
    requirement: KIT-02
    verification:
      - kind: other
        ref: "go list -deps ./kit/session | grep -cE 'ass-guard-agent/internal/(ecosys|perm)$' -> 0; the gate batteries (deny/allow/ask/unmatched, MCP subjects, hook-verdict head) pass unchanged"
        status: pass
      - kind: unit
        ref: "go test ./kit/session/ -count=1 -run 'TestGate' -> green (fakePermStore adapts through testRuleSet at the wiring site)"
        status: pass
    human_judgment: false
  - id: D3
    description: "kit/runtime/catalog.go defines CommandCatalog with the consumption-derived method set (14 methods, all live sites), the kit Command record, contract doc comments, and the emptyCatalog nil degradation"
    requirement: KIT-01
    verification:
      - kind: other
        ref: "grep -c '^	[A-Z]' catalog.go interface body = 14 methods, each with the who-implements/nil-degrades contract comment; emptyCatalog implements the full set returning documented empty values"
        status: pass
      - kind: unit
        ref: "nil-degradation exercised by every bare runner battery (no SetCatalog): builtins resolve, unknown slash names stay plain text, /help renders the builtin-only chain"
        status: pass
    human_judgment: false
  - id: D4
    description: "kit/runtime's dependency set contains NO internal/ecosys and NO internal/openspec (edges 2, 3, and the 25-05 LoadCommandRegistry mutability residual severed)"
    requirement: KIT-03
    verification:
      - kind: other
        ref: "go list -deps ./kit/runtime | grep -cE 'ass-guard-agent/internal/(ecosys|openspec)$' -> 0 (grep exit 1); remaining internal set exactly {coreexec, perm, sandbox, sched, tasks} — sched is the recorded coreexec transitive (25-07)"
        status: pass
    human_judgment: false
  - id: D5
    description: "The catalog adapter is the only place ecosys values map to kit types; no kit signature names an app type; discovery, precedence, and OpenSpec hosting are app-side"
    requirement: KIT-03
    verification:
      - kind: other
        ref: "internal/acpserve/catalog_adapter.go holds the ONE mappings (kitAgentDef/kitCommand/hooksAdapter); kit signatures reference only kit types (session.AgentDef, session.Hooks, mcp.ServerConfig, toolcat.Stub, runtime.Command/CatalogEntry/MemoryFile)"
        status: pass
      - kind: unit
        ref: "go test ./internal/acpserve/ -count=1 -skip TestPermissionsE2E -> ok 27.3s (serve/simulator/zeroconfig batteries green through the adapter)"
        status: pass
    human_judgment: false
  - id: D6
    description: "Discovery degradation arms preserved app-side and pinned; acpserve.Run statement order preserved (discovery step at LoadCommandRegistry's old position)"
    requirement: KIT-03
    verification:
      - kind: other
        ref: "git show 9ac3aab -- internal/acpserve/acp_serve.go: the ONLY change in Run is the one-statement swap runner.LoadCommandRegistry() -> runner.SetCatalog(loadCommandCatalog(opts.WorkDir)) at the same position"
        status: pass
      - kind: unit
        ref: "TestExpansionDegradedRegistry (runner_battery): broken tree -> twin logs 'command registry load failed (continuing without slash expansion)' -> raw-text turn proceeds — the arm pins identically through the twin"
        status: pass
    human_judgment: false
  - id: D7
    description: "D-20 equivalence: split gates green differentially; lint/race vs the fresh 5a68cfc baseline — zero new findings, identical race failing family; ledger 1397 == 1397"
    requirement: KIT-03
    verification:
      - kind: other
        ref: "vet 0 / build (CGO=0) 0 / full-repo lint 589 vs baseline 597 (net -8, zero new; the only diff hunks are type-name drift on pre-existing findings) / race failing set == the documented family by name (PermissionsE2E, LiveInstalledPluginsProbe, 4xRunSuite — each reproduced IDENTICALLY at the baseline worktree; RescanConcurrency green this run); ledger kit 807 + internal 557 + cmd 33 = 1397"
        status: pass
    human_judgment: false

duration: 72 min
completed: 2026-09-10
status: complete
---

# Phase 25 Plan 06: kit/session Neutral Types + CommandCatalog Injection Summary

**The ecosystem edges severed — kit/session speaks only kit vocabulary (AgentDef/Hooks/HookOutcome plus the assigned RuleSet view that also kills session→perm), and the kit consumes the command/skill/plugin world exclusively through its injected CommandCatalog with .claude/ discovery + OpenSpec hosting app-side in internal/acpserve — kit/runtime's dependency closure contains zero ecosys and zero openspec, all batteries green, lint/race identical to the fresh baseline, ledger 1397**

## Performance

- **Duration:** 72 min (2026-09-10T22:22Z → 23:34Z)
- **Started:** 2026-09-10T22:22:31Z
- **Completed:** 2026-09-10T23:34:00Z
- **Tasks:** 2/2
- **Files modified:** 43 in plan scope (6 created, 37 modified; 21 of them the mechanical test retargets)

## Accomplishments

- Task 1 (`097711e`): kit/session/types.go — the kit-neutral vocabulary with per-type mirror-or-promote justifications. AgentDef carries exactly the consumed fields (Name/Description/Tools/Model/Prompt — T-25-27 minimality); the Hooks interface joins the lifecycle Fire with the PreToolUseVerdict/PostToolUse pair; HookOutcome/HookVerdict mirror the outcome/verdict contracts. gate.go's verdict head and rule evaluation retargeted to the kit enums; the MCP-name canonicalization default became the kit-side mirror. The 25-03-assigned session→perm residual went in the same commit: the kit RuleSet interface + RuleVerdict enum + a composition-side kitRuleSet adapter in runtime.go. kit/session's internal dependency set is now EMPTY — zero internal/* at any depth.
- Task 2 (`9ac3aab`): the D-17 inversion. kit/runtime/catalog.go defines the CommandCatalog seam (14 consumption-derived methods), the kit Command record with the promoted Expand substitution, the promoted ParseInvocation/ParseMentions parsers (the nil-degradation invariant), and the emptyCatalog never-panic degradation. runtime.go/commands.go/rescan.go rewired: the chain builds over []CatalogEntry, resolveSlashCommand fetches bodies through LookupCommand, the rescan rides Rediscover, /memory //mcp //doctor read the catalog. LoadCommandRegistry, the registry/mutability/mcpServers fields, CommandRegistry/registrySnapshot, and the Task-1 temporary mappings are deleted. internal/acpserve gained discover.go (the relocated discovery step, degradation arms verbatim) and catalog_adapter.go (the ONE app→kit mapping site); acpserve.Run swaps exactly one statement at LoadCommandRegistry's old position.

## Task Commits

1. **Task 1: kit/session neutral types — AgentDef, Hooks, HookOutcome (+ the session→perm residual)** — `097711e` (feat; 15 files)
2. **Task 2: CommandCatalog injection (D-17) — kit interface, app-side discovery, adapter** — `9ac3aab` (feat; 25 files)

**Plan metadata:** this docs commit.

## Verification Evidence (all re-run at final HEAD)

Task 1 acceptance criteria:
- types.go defines AgentDef, Hooks (lifecycle Fire + PreToolUseVerdict/PostToolUse), HookOutcome with the mirror-or-promote justification per type — **PASS** (doc comments state the choice per type; the Hooks interface is the Fire + verdict + observation triple the pipeline consumes)
- kit/session's dependency set contains no internal/ecosys (edge 7 severed) — **PASS** (go list -deps: the internal set is EMPTY — ecosys AND perm AND everything else)
- All session Fire/hook call sites compile against the kit interface; relocated batteries pass with assertion-preserving retargets — **PASS** (kit/session + kit/runtime suites ok; the 7 session test files retarget via fixture type swaps + the adapt_twin_test twins; the real script-hook runners and perm rule sets adapt at the wiring site)

Task 2 acceptance criteria:
- catalog.go defines CommandCatalog with the consumption-derived method set and the kit Command record, all with contract doc comments — **PASS** (14 methods; the set extension beyond the plan's 9 is Deviation 1 below)
- kit/runtime's dependency set contains NO internal/ecosys and NO internal/openspec — **PASS** (go list -deps grep exit 1; remaining internal set exactly {coreexec, perm, sandbox, sched, tasks})
- The degradation arm (discovery failure → expansion off, plain-text turns) preserved in discover.go and pinned by a relocated test — **PASS** (the arms are verbatim — log lines byte-identical; TestExpansionDegradedRegistry pins the raw-text turn through the twin)
- The catalog adapter is the only place ecosys values map to kit types; no kit signature names an app type — **PASS**
- acpserve.Run statement order preserved (discovery step at LoadCommandRegistry's old position); mise ci green — **PASS** (one-statement swap, diff shown; mise ci green DIFFERENTIALLY: vet 0, build 0, lint 589<597 with zero new findings, race failing family identical to the fresh baseline — see Issues for the environmental flake proof)

Plan-level verification:
- kit/session and kit/runtime import neither ecosys nor openspec (edges 2, 3, 7 severed + the residual closed) — **PASS** (final-HEAD dep-list evidence above)
- The CommandCatalog method set covers every documented consumption site; nothing more (D-12) — **PASS with the recorded extension** (14 methods, every one a live site; Deviation 1)
- Discovery degradation arms preserved app-side and pinned by tests — **PASS**
- mise ci green; composition statement order preserved — **PASS differentially** (identical race failing family; zero new lint findings vs the fresh baseline)

Assigned residuals (orchestrator):
1. **openspec residual (from 25-05)** — CLOSED: kit/runtime's openspec import existed exactly as LoadCommandRegistry's mutability read; the read now happens app-side (discover.go loads oscfg.CommandMutability; the adapter's Mutating(key) resolves against openspec.MutabilityMutating — T-25-29: no mutability literal kit-side). `go list -deps ./kit/runtime | grep -c openspec` → 0.
2. **session→perm (from 25-03's inventory)** — SEVERED: GateDeps.Rules/PreToolUseVerdict speak kit types (RuleSet/HookVerdict); the gate's MCP-subject default is the kit canonicalToolName mirror; `go list -deps ./kit/session | grep -cE 'internal/(ecosys|perm)$'` → 0. The runtime→perm edge REMAINS (per the ledger, a later plan owns it).

Edge ledger after this plan (go list evidence): kit/runtime → {coreexec (25-07), perm, sandbox, tasks, sched-via-coreexec (25-07)}; kit/session → {} (clean); enginebridge → clean (25-05). Of the 12 pass-2 edges: 7 now severed (acp, sched-direct, learning ×2, openspec-half, session→ecosys, session→perm) + ecosys/openspec runtime edges gone here.

D-20 ledger: kit 807 + internal 557 + cmd 33 = **1397 == 1397** (the twin files add zero Test functions — helpers only, the 25-05 precedent).

TDD gate: `task.is-behavior-adding` = false (no tdd attr, no behavior block) — the plan's pre-recorded TDD-ineligibility held for both tasks; the relocated suites are the contract (the 25-04/25-05 precedent).

Threat register dispositions (all mitigated as planned):
- T-25-25 (hook authority): the kit Hooks interface is deny/lifecycle-shaped — Fire/PreToolUseVerdict/PostToolUse only; no allow-granting method exists kit-side; USER-scope-only allow resolution stays app-side (the adapter demotes before the verdict crosses). PASS.
- T-25-26 (discovery validation lost): discover.go relocates the arms verbatim; the expansion/degradation batteries pin behavior. PASS.
- T-25-27 (AgentDef over-carriage): the mirror carries exactly the consumed fields (enumerated in the doc comment — Tools/Prompt from dispatch, Model from the 20-03 planner, Name/Description for the advisory surfaces). PASS.
- T-25-28 (nil catalog panic): emptyCatalog returns documented empty values; bare-runner batteries exercise the path. PASS.
- T-25-29 (mutability vocabulary drift): Mutating(key) bool is queried from the catalog; no openspec literal exists in kit code (grep MutabilityMutating kit/ → 0). PASS.

## Files Created/Modified

See key-files (frontmatter). 43 files in plan scope: 6 created, 37 modified (21 mechanical test retargets across kit/session and kit/runtime).

## Decisions Made

- The five key-decisions in the frontmatter (promoted pure parsing; the 14-method consumption-derived set; the perm severance placement; the kind-carrying LookupCommand; the kit-side agent-hint stamp).
- The catalog twin (catalog_twin_test.go) over an importable shared adapter: white-box batteries cannot import acpserve (test-cycle), and the plan's artifact letter names internal/acpserve/catalog_adapter.go as the adapter's home — the twin is the 25-05-precedented shape at larger scale.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - blocking] The CommandCatalog method set is 14, not the plan's 9**
- **Found during:** Task 2 (the verify clause is unsatisfiable at 9 methods)
- **Issue:** The plan's must_hive enumerated the runtime.go consumption sites only (ParseInvocation, Lookup, Mutability, SkillListing, AgentListing, Agents, Hooks, SkillExecute, MCPLayers). commands.go's chain machinery (buildChain/installRegistry/resolveSlashCommand/skillBodyEmpty), rescan.go's Discover, the @-mention parser, memory injection/files, and /doctor's registry counts are equally runtime→ecosys edges — `go list -deps` cannot reach zero without covering them.
- **Fix:** The seam covers every live consumption site (Entries, Rediscover, LookupCommand, SkillBody, Mutating, Agents, Hooks, SkillExecute, SkillListing, AgentListing, MemoryInjection, MCPLayers, MemoryFiles, Counts). must_hive truth #2's PRINCIPLE ("ONLY through its CommandCatalog interface") holds exactly; the enumeration was illustrative.
- **Files modified:** kit/runtime/catalog.go (the interface), commands.go, rescan.go, runtime.go
- **Verification:** dep-list zero-ecosys proof; all batteries green
- **Committed in:** `9ac3aab`

**2. [Rule 3 - blocking] ParseInvocation/ParseMentions/Expand promoted kit-side, not catalog methods**
- **Found during:** Task 2 (the nil-degradation design)
- **Issue:** The plan listed ParseInvocation as a catalog method. But class-B command detection, /undo's mid-turn routing, and the builtin chain must parse identically whether or not discovery ever succeeded — a nil-catalog parse failure would break builtins for bare runners (a behavior change the plan never intended).
- **Fix:** The three pure text parsers promote verbatim kit-side (zero ecosystem knowledge — the file documents the promotion); the catalog owns only registry/ecosystem state. The app-side originals remain for ecosys's own consumers.
- **Files modified:** kit/runtime/catalog.go
- **Verification:** bare-runner batteries (no SetCatalog) resolve builtins and route /undo identically; the expansion/mention batteries pin the promoted parsers byte-for-byte
- **Committed in:** `9ac3aab`

**3. [Rule 2 - assigned residual] session→perm severed inside Task 1**
- **Found during:** Task 1 (the orchestrator's assigned-residuals instruction; 25-03's inventory)
- **Issue:** The plan's files/must_hives do not mention perm; the edge was assigned to 25-06 by 25-03's drift record. gate.go's rules/verdict/canonicalization vocabulary was the whole surface.
- **Fix:** kit RuleSet/RuleVerdict types + the kit canonicalization mirror in types.go; GateDeps.Rules/PreToolUseVerdict retyped; a composition-side kitRuleSet adapter in runtime.go (the runtime keeps its own perm edge — a later plan's).
- **Files modified:** kit/session/{types,gate}.go, kit/runtime/runtime.go, test retargets
- **Verification:** kit/session dep set EMPTY; the gate batteries green with unchanged assertions
- **Committed in:** `097711e`

**4. [Rule 1 - lint fallout] new-file lint findings (ireturn/interfacebloat/hugeParam/funcorder/exhaustive/nlreturn/lll/forcetypeassert/unused)**
- **Found during:** per-task lint differentials vs the fresh baseline worktree
- **Issue:** First drafts of types.go/catalog.go/catalog_adapter.go/the twins plus the fixture retargets tripped ~25 findings across nine linters.
- **Fix:** restructured (pointer params, checked assertions via a testCatalogOf helper, moved methods for funcorder, explicit exhaustive cases, deleted the unused registryDiff); justified nolints only where the interface IS the seam (ireturn/interfacebloat). Final differentials: kit/runtime net -7 with zero new, kit/session net -1 with zero new, acpserve zero new; full-repo 589 vs baseline 597.
- **Files modified:** the six new files + the retargeted test files
- **Committed in:** `097711e` / `9ac3aab`

---

**Total deviations:** 4 auto-fixed in-plan (2 blocking scope extensions the verify clause forced, 1 assigned residual, 1 lint fallout), 0 architectural stops
**Impact on plan:** Plan goal fully achieved — both ecosys edges plus the openspec residual severed, session→perm closed, the kit ecosystem-blind by injection, every gate green-or-identical-to-baseline.

## Issues Encountered

- **TestPermissionsE2E fails under load on this machine — PRE-EXISTING, proven at baseline:** the failure (permissions_e2e_test.go:592 "simulator: timed out waiting for a frame (seen=11)", 10.27s) reproduces IDENTICALLY in the detached baseline worktree at 5a68cfc — both under the full acpserve suite AND in isolation there — while passing in isolation on earlier runs of both trees. It is environmental (the serve stderr shows the probe reads the operator's real ~/.claude installed-plugins state). The race failing set matches the documented 25-05 family by name with nothing new; this is recorded for 25-09's D-20 close-out, not fixed here (scope discipline).
- The kit/runtime chain fixture retarget surfaced one real behavioral gap in the first draft (agent winners lost their dispatch hint because the hint is a kit convention, not discovery data) — caught by TestCommandChainAdvertisementShape, fixed by the kit-side stamp (key-decisions #5).
- Pre-existing WINDOWS.md row-29 integrity mismatch — left untouched (the 25-05 precedent).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Ready for 25-07 (SessionToolkit — coreexec): kit/runtime's remaining internal set is exactly {coreexec, perm, sandbox, tasks, sched-via-coreexec}; severing coreexec also kills the last transitive sched path. The catalog twin pattern generalizes to the toolkit twin.
- Ready for 25-08/25-09: the white-box test disposition now has two twin precedents (engine_setup, catalog); the D-19 gate can rely on kit/session and kit/runtime being PRODUCTION-clean of ecosys/openspec/perm(app-side mapping only). The watch-root path literals in rescan.go (.claude/.ass-guard names) remain kit-side — import-clean but a documented D-18-letter observation for 25-09's review.
- Edge ledger: 5 of 12 pass-2 edges remain (coreexec, perm×1, sandbox, tasks, + the sched transitive riding coreexec).

## Self-Check: PASSED

- kit/session/types.go, kit/runtime/catalog.go, internal/acpserve/discover.go, internal/acpserve/catalog_adapter.go, kit/runtime/catalog_twin_test.go, kit/session/adapt_twin_test.go exist on disk — FOUND
- Commits 097711e, 9ac3aab present in git log — FOUND
- Ledger 1397 == 1397 — VERIFIED (kit 807 + internal 557 + cmd 33)
- Dep severance re-verified at final HEAD: kit/session internal set EMPTY; kit/runtime ecosys 0 AND openspec 0 (grep exit 1 on both verify clauses); remaining set exactly {coreexec, perm, sandbox, sched, tasks}
- All acceptance criteria re-run with PASS evidence above (the two residuals dispositioned with dep-graph evidence)

---
*Phase: 25-seed-001-kit-extraction-strictly-last*
*Completed: 2026-09-10*
