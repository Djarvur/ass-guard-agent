# Phase 15: internal/runtime Carve (Step 0) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-26
**Phase:** 15 - internal/runtime Carve (Step 0)
**Areas discussed:** Test relocation, Composition surface, Carve scope, Export naming

---

## Test Relocation

| Option | Description | Selected |
|--------|-------------|----------|
| Move all runner tests | ~10 test files move verbatim into internal/runtime white-box tests; cmd keeps stdio E2E only | ✓ |
| Split by kind | White-box moves, E2E stays and rewrites against exported surface | |
| Keep in cmd | All tests stay in package main | |

**User's choice:** Move all runner tests
**Notes:** Follow-ups: mixed files split by subject (runner tests move, acp.NewServer handler tests stay); small helpers duplicated across packages rather than a testutil pkg; cmd keeps only true stdio E2E (e2e_opsx*, integration).

---

## Composition Surface

| Option | Description | Selected |
|--------|-------------|----------|
| Config struct + New | runtime.NewRunner(RunnerConfig) mirrors today's literal; fields stay unexported on Runner | ✓ |
| Exported fields | &runtime.Runner{...} freezes field names as API | |
| Functional options | WithProfile etc. — new API surface during verbatim phase | |

**User's choice:** Config struct + New
**Notes:** Thin constructor (struct fill only, startup steps invoked explicitly by caller). RunnerConfig mirrors current field names exactly.

### Composition Surface (continued)

| Option | Description | Selected |
|--------|-------------|----------|
| Keep runACPServe in cmd | runACPServe stays, uses runtime.* qualifiers | |
| Move runACPServe too | Moves out of cmd | ✓ |

**User's choice:** "move, but this is not the part of the kit, separate package" (free text)
**Notes:** Operator redirected: runACPServe leaves cmd but must NOT live inside internal/runtime. Pinned to `internal/acpserve`; all non-runner wiring follows (profile-dir helpers, perm warnings, audit mirror, redactorAdapter, serveOptions→Options).

### Composition Surface (other cmd files)

| Option | Description | Selected |
|--------|-------------|----------|
| CLI/setup helpers stay | checkpoint/learning/modelrouting/provider_factory/parity stay in cmd | |
| Provider setup moves too | Only factory+modelrouting to acpserve | |

**User's choice:** "again it should move but to the separate package" (free text)
**Notes:** All five files leave cmd, each to its own package-by-concern (not one shared package). Cobra boundary follow-up: only run*/resolve* logic moves; cobra command definitions stay in cmd. serveOptions + goconst constants: Options exported from acpserve; constants split by destination with duplication allowed.

---

## Carve Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Runner + direct deps | Everything needed to compile: struct, methods, parkedChain, adapters, ask wiring | ✓ |
| Core methods only | Struct + Run/runOneTurn/sessionFor; helpers left behind (impossible for unexported methods across packages) | |

**User's choice:** Runner + direct deps
**Notes:** All 7 adapter types confirmed moving (stubCatalogExec, acpDispatcher, patternNextPrompter, hookSessionTurnRunner, hookSessionBoundaryOpener, realCommandRunner, engineTurnRunnerAdapter).

### Carve Scope (packaging)

| Option | Description | Selected |
|--------|-------------|----------|
| Single pkg, split later | One internal/runtime until Phase 25 | |
| Sub-packages now | 3-way split at carve time | ✓ |

**User's choice:** Sub-packages now
**Notes:** 3-way: internal/runtime (core) + internal/runtime/enginebridge (7 adapters + engine setup) + internal/runtime/cron (9 cron methods + scheduler loop). acpserve composes all three.

### Carve Scope (enginebridge seam)

| Option | Description | Selected |
|--------|-------------|----------|
| Exported accessors | Small accessor methods / Fields struct on Runner | |
| Mirror config into bridge | Bridge struct mirrors needed fields; acpserve fills both | ✓ |
| Collapse packages | Drop enginebridge, keep fewer packages | |

**User's choice:** Mirror config into bridge
**Notes:** Divergence risk accepted deliberately over exporting runner internals.

### Carve Scope (loose ends)

| Option | Description | Selected |
|--------|-------------|----------|
| Func field + adapter rides | emitFor stays RunnerConfig func field; checkpointerAdapter moves into internal/runtime | ✓ |
| Duplicate adapter | Two adapters for one seam | |

**User's choice:** Func field + adapter rides
**Notes:** srv.Emitter injected after NewRunner same as today; cobra part of checkpoint.go still goes to its own package per composition decisions.

---

## Export Naming

| Option | Description | Selected |
|--------|-------------|----------|
| runtime.Runner + New | Go stutter convention; clean package-qualified reads | ✓ |
| runtime.TurnRunner | Mirrors acp.TurnRunner name-for-name — confusion risk | |
| SessionTurnRunner | Full original name exported | |

**User's choice:** runtime.Runner + New
**Notes:** Confirmed RunnerConfig + New(RunnerConfig) *Runner; enginebridge/cron type names at Claude discretion. Minimal method exports (only what compiles today, no YAGNI pre-exports). Kit-facing package doc planted now: internal/runtime = future SEED-001 kit core, acpserve NOT kit, no ACP-specific words in runtime API.

---

## Deferred Ideas

None — discussion stayed within phase scope.
