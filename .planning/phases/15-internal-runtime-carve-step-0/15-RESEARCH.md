# Phase 15: internal/runtime Carve (Step 0) - Research

**Researched:** 2026-08-26
**Domain:** Go package refactor — mechanical verbatim relocation of `sessionTurnRunner` + serve wiring + CLI support out of `cmd/ass-guard`
**Confidence:** HIGH (codebase-grounded; every structural claim verified by direct file reads this session)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Test Relocation**
- **D-01:** All runner-touching test files (~10 files constructing `sessionTurnRunner{}` directly) move verbatim into `internal/runtime/*_test.go` as same-package white-box tests.
- **D-02:** Mixed files split by subject: runner unit tests move; tests exercising the `acp.NewServer` handler layer stay in cmd (e.g. part of acp_serve_test.go's 27 tests).
- **D-03:** Small shared test helpers (fake providers, bus fixtures) are duplicated across packages rather than extracted into a shared testutil package — no new surface during carve.
- **D-04:** cmd keeps only true stdio E2E suites (spawn binary + JSON-RPC): e2e_opsx*, integration.

**Composition Surface**
- **D-05:** Constructor shape: `runtime.NewRunner(RunnerConfig) *Runner`. RunnerConfig mirrors today's field names exactly (Bus, Profile, WorkDir, MakeProvider, …). Thin constructor — struct fill only; caller invokes loadCommandRegistry / setupEngine / startScheduler explicitly, preserving today's startup order. — **Reversibility:** costly — field names become kit API consumed by acpserve and all moved tests; renaming later touches every construction site plus Phase 25's public surface.
- **D-06:** runACPServe moves OUT of cmd but NOT into the kit package — new separate package `internal/acpserve`; cmd calls `acpserve.Run(ctx, in, out, stderr, opts)`. Serve concerns must not leak into the future kit. (Operator: "move, but this is not part of the kit, separate package.")
- **D-07:** ALL non-runner wiring in acp_serve.go moves to internal/acpserve verbatim: profile-dir helpers, perm warnings, audit mirror, redactorAdapter. serveOptions exported there as `Options`; cmd builds Options from flags.
- **D-08:** Other cmd non-test files leave cmd too, each to its own package-by-concern (operator: "again it should move but to the separate package"): checkpoint.go → its own pkg, learning_cmd.go → its own pkg, modelrouting.go → its own pkg, provider_factory.go → its own pkg, parity.go → its own pkg. Package per concern, not one combined package.
- **D-09:** Cobra boundary: only run*/resolve* logic functions move into the new packages; cobra command definitions stay in cmd and call into them. cmd remains the CLI-shape owner (root command + subcommand wiring).
- **D-10:** goconst_constants.go splits by destination: each new package takes the subset of constants it uses; verbatim duplication across packages allowed during carve.

**Carve Scope**
- **D-11:** internal/runtime receives everything `sessionTurnRunner` needs to compile: runner struct + methods, `parkedChain`, `advisoryNote`, askBroker callback wiring inside sessionFor, expandUserBlocks/invocationFor, park/idle-tracking state, advisory dedupe.
- **D-12:** All 7 package-local adapter types move with the runner: stubCatalogExec, acpDispatcher, patternNextPrompter, hookSessionTurnRunner, hookSessionBoundaryOpener, realCommandRunner, engineTurnRunnerAdapter.
- **D-13:** Sub-package split NOW, 3-way (operator choice over single package): `internal/runtime` (runner core), `internal/runtime/enginebridge` (7 adapters + engine setup helpers), `internal/runtime/cron` (cron_wiring.go's 9 methods re-homed + scheduler loop). acpserve composes all three.
- **D-14:** enginebridge seam = MIRROR CONFIG (operator choice over exported accessors): enginebridge defines its own struct mirroring the runner fields the adapters need; acpserve fills both from one source. Divergence risk accepted deliberately.
- **D-15:** cron methods ride with the runtime family (not left at wiring layer) — they are methods on the runner today; they land in internal/runtime/cron.
- **D-16:** `emitFor` stays a RunnerConfig func field — acpserve injects `srv.Emitter` after NewRunner, same as today's `runner.emitFor = srv.Emitter`.
- **D-17:** checkpointerAdapter moves into internal/runtime beside its sessionFor use site; checkpoint.go's cobra/list/restore logic goes to its own package per D-08/D-09.

**Export Naming**
- **D-18:** Exported name: `runtime.Runner`, constructor `runtime.NewRunner(cfg)`. Go stutter convention (not `runtime.TurnRunner` — avoids same-name-as-acp confusion; not `SessionTurnRunner` — too long).
- **D-19:** Method set unchanged: Run, CloseSession, WaitChainIdle keep names; sessionFor and friends stay unexported. Minimal exports — only what cmd/acpserve needs to compile. No pre-emptive exports for later phases (YAGNI).
- **D-20:** Kit-facing doc now: internal/runtime package doc states the kit ambition — this becomes SEED-001 kit core at Phase 25; acpserve is NOT kit; no ACP-specific words in runtime API naming.

### Claude's Discretion
- Exact per-test assignment when splitting mixed test files (subject-based judgment).
- Constant-by-constant placement within the "each package takes what it uses" rule.
- File layout within each new package (one file per concern or merged).

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| RUNT-01 | `sessionTurnRunner` carved verbatim from `cmd/ass-guard/acp_serve.go` into `internal/runtime`; cmd composes it; zero behavior change (`mise ci` proves equivalence) | Full file inventory + line ranges below; compile-closure task decomposition; `mise ci` gate mapped as the equivalence proof (Validation Architecture); CLI-contract invariance identified as the operator-visible behavior surface |
</phase_requirements>

## Project Constraints (from CLAUDE.md / project conventions)

No `CLAUDE.md` exists at repo root or `.claude/` (checked this session). Governing conventions come from `.golangci.yml` (strict lint, quoted below), `.mise.toml` (CI gate), and the codebase's own disciplines — transport discipline (stdout carries only ACP frames), graceful-degradation-at-startup, `//nolint` comments riding specific lines. Treat all three with CONTEXT-decision authority.

## Summary

Phase 15 relocates ~2,400 lines of production Go (2115-line `acp_serve.go` + 258-line `cron_wiring.go` + parts of five CLI-support files) out of `cmd/ass-guard` into `internal/runtime` (3-way split), `internal/acpserve`, and five package-per-concern CLI homes, with ~8,000 lines of tests redistributed by subject. The move target is precisely bounded: the `sessionTurnRunner` struct spans `acp_serve.go:441-544` with methods to line 2095, plus 9 methods in `cron_wiring.go`, plus `checkpointerAdapter` in `checkpoint.go:22-30`. The wire seam is stable — `internal/acp/server.go:31-42` defines `TurnRunner`/`ChunkEmitter`/`SessionCloser` interfaces the carved `runtime.Runner` satisfies unchanged.

Two findings dominate planning. **First, a Go language constraint collides with D-13+D-15 as literally stated:** methods on a type must be declared in that type's package ([CITED: go.dev/ref/spec#Method_declarations]), so the 9 cron methods cannot land in `internal/runtime/cron` while remaining methods on `runtime.Runner` — and four of them are called from runner-core code (`Run`, `runOneTurn`, `sessionFor`), creating a guaranteed import cycle if they move naively. The mechanical resolutions (embedded state-owner type, or a core-file/cron-package method split) are enumerated below as the phase's #1 planning decision. The same visibility mechanics constrain D-12+D-14: adapters like `engineTurnRunnerAdapter` reach the runner's unexported members (`a.r.invocationFor`, `a.r.sessMu`, `a.r.automationProvenance` — acp_serve.go:1757-1798), so adapter *type definitions* can live in enginebridge but their *construction* must stay inside package runtime, handed func values across via exported bridge fields/constructors. **Second, a D-07/D-11 tension:** `redactorAdapter` is assigned to acpserve by D-07 but is *constructed inside `sessionFor`* (acp_serve.go:1157,1160) — the runner core needs a copy (verbatim duplication, consistent with D-03/D-10) or the decision needs one-line reinterpretation.

Beyond those two, the risks are enumerable and mechanical: test files construct the runner with unexported fields (forcing D-01's same-package placement — confirmed necessary), `TestMain` uniqueness (mcp_tracer_test.go:35), lint path-exclusions tied to `cmd/.*` that stop applying to moved files, two cobra-typed signatures needing de-cobra-ing (`runACPServeCmd` at :198, `resolveProfilesDir` at :298), and a strict startup-order invariant (`srv.Emitter` injection between `NewServer` and `startScheduler`, acp_serve.go:397-412). Verification is already solved: the standing `mise ci` gate (vet + lint + CGO_ENABLED=0 build + `go test -race ./...`) plus a 130-test-function baseline ledger and `git diff --color-moved` review recipe prove the pure-relocation bar.

**Primary recommendation:** Plan the carve as one serial compile-closure wave (runtime family + acpserve + test relocation cannot individually reach green), followed by an independently-verifiable CLI-support wave of five small extractions, closed by a docs/verification wave. Resolve the cron-mechanism and redactorAdapter questions in the plan itself, not at execution time.

## Architectural Responsibility Map

Adapted to this codebase's tiers (CLI binary → serve composition → domain packages):

| Capability | Primary Tier (today) | Primary Tier (after Phase 15) | Rationale |
|------------|---------------------|-------------------------------|-----------|
| Turn execution (Run/runOneTurn/sessionFor) | cmd/ass-guard (package main) | internal/runtime (runner core) | D-11/D-13 — the kit seed owns turn lifecycle |
| Engine/hook adapters | cmd/ass-guard | internal/runtime/enginebridge | D-12/D-13/D-14 — mirror-config seam |
| Cron/scheduler loop | cmd/ass-guard (cron_wiring.go) | internal/runtime/cron | D-13/D-15 — modulo the method-placement mechanism (Open Question 1) |
| ACP serve composition (flags→Options, bus, audit mirror, provider factory call, server construction) | cmd/ass-guard | internal/acpserve | D-06/D-07 — explicitly NOT kit |
| CLI shape (cobra commands, flags) | cmd/ass-guard | cmd/ass-guard (stays) | D-09 — cmd remains CLI-shape owner |
| Checkpoint/learning/model-routing/parity CLI logic | cmd/ass-guard | one package per concern | D-08/D-09 — run*/resolve* logic only |
| ACP wire protocol | internal/acp (untouched) | internal/acp (untouched) | Interface seam already correct |

## Standard Stack

**No new dependencies. This phase adds zero packages.** The carve uses only what is pinned today.

| Tool | Version | Purpose | Why Standard |
|------|---------|---------|--------------|
| Go | 1.26 (mise-pinned; 1.26.1 installed) | language/toolchain | `[VERIFIED: .mise.toml:2 'go = "1.26"'; mise exec -- go version → go1.26.1]` |
| golangci-lint | 2.x strict (`default: all`; 2.12.2 installed) | the lint half of the gate | `[VERIFIED: .mise.toml:3; golangci-lint version output this session]` |
| mise tasks | vet/lint/build/test | THE equivalence gate | `[VERIFIED: .mise.toml [tasks.ci] depends = ["vet", "lint", "build", "test"]]` |
| spf13/cobra | existing dep | stays in cmd only | `[VERIFIED: go.mod require block; acp_serve.go:22]` |
| git `--color-moved` | system git | reviewability of the pure-relocation diff | `[ASSUMED]` standard git feature (≥2.18), not exercised this session |

**Installation:** nothing. **Explicitly forbidden:** introducing any module during the carve (would break the pure-relocation reviewability criterion).

## Package Legitimacy Audit

Not applicable — this phase installs no external packages. Existing dependencies are already vendored/pinned in `go.mod` and pass the standing gate.

## Architecture Patterns

### System Architecture Diagram (post-carve data flow)

```
Zed ──spawn──▶ ass-guard acp serve          (cmd/ass-guard — cobra shell, D-09)
                 │  parses flags ──builds──▶ acpserve.Options   (D-07)
                 ▼
            acpserve.Run(ctx, in, out, stderr, opts)           (internal/acpserve)
                 │  event.NewBus → profile load → setupModelRouting
                 │  perm warnings → bodyStore → startAuditMirror
                 ▼
            runtime.NewRunner(RunnerConfig) ◀── filled from ONE source (D-05/D-14)
                 │        +
            enginebridge mirror-config   ◀── filled alongside (D-14)
                 │
                 ├─▶ runner.loadCommandRegistry()      (explicit, order preserved, D-05)
                 ├─▶ runner.setupEngine()              (explicit, degrades gracefully)
                 ▼
            acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))
                 │                                      (internal/acp — unchanged)
                 ├─▶ runner.emitFor = srv.Emitter       (WINDOWS #3 ordering, D-16)
                 ├─▶ runner.startScheduler(ctx)         (cron family, D-15)
                 ▼
            srv.Serve(ctx) ──frames──▶ stdout           (transport discipline)
```

Startup order is a behavioral invariant: loader → factory → warnings → bodyStore → mirror → NewRunner → loadCommandRegistry → (optional) setupEngine → NewServer → emitFor injection → schedule-store open → startScheduler → closeAllSessions-on-ctx goroutine → Serve.

### Recommended Project Structure

```
cmd/ass-guard/               # CLI shape only (D-09): main.go, root/acp/checkpoint/
                             #   learning/model-routing/profile/parity cobra shells,
                             #   true-binary-spawn E2E (see Pitfall 9 for the e2e_opsx nuance)
internal/
├── runtime/                 # D-18/D-20 kit seed: Runner, RunnerConfig, sessionFor,
│   │                        #   expandUserBlocks, park/idle state, advisory dedupe,
│   │                        #   same-package white-box *_test.go (D-01)
│   ├── enginebridge/        # D-12/D-13/D-14: 7 adapter types + engine setup helpers,
│   │                        #   mirror-config struct, constructors fed by runtime core
│   └── cron/                # D-13/D-15: scheduler loop family (mechanism: Open Question 1)
├── acpserve/                # D-06/D-07: Run(), Options, profile-dir helpers, perm
│                            #   warnings, audit mirror (+ redactorAdapter disposition: OQ 2)
├── checkpointcmd/           # D-08 (names illustrative): runCheckpointList/Restore
├── learningcmd/             # D-08: runLearningList/Revert, resolveLearnedPath
├── modelroutingcmd/         # D-08: emitResolveHuman/JSON, describeCapabilities
├── providerfactorycmd/      # D-08: setupModelRouting/setupProviderFactory/loadModelRoutingFactory,
│                            #   globalConfigPath/projectConfigPath/warnLooseConfigPerm
└── paritycmd/               # D-08: zcode-version/drift run logic
```

Naming of the five CLI packages is planner discretion (CONTEXT fixes the *shape*: one package per concern, not combined).

### Pattern 1: Same-package white-box test relocation (D-01)

**What:** Moved tests keep `package runtime` (not `runtime_test`) because they fill unexported struct fields.
**When to use:** Every test that writes a composite literal like `&sessionTurnRunner{bus: …, makeProvider: …}`.
**Evidence:** 21 runner literals across 12 test files (counts per file in the inventory below); unexported fields make external test packages impossible. `[VERIFIED: acp_serve.go:441-544 — all fields unexported; grep counts this session]`

### Pattern 2: Cross-package adapter construction (the D-14 mechanics)

**What:** Adapter *types* live in enginebridge; their *construction sites* stay in runtime-core methods (`setupEngine`, `runOneTurn`, `sessionFor`), passing func values derived from unexported runner members.
**Why it must work this way:** `engineTurnRunnerAdapter` reaches `a.r.invocationFor(prompt)` / `a.r.expandUserBlocks(...)` / `a.r.sessMu` / `a.r.automationProvenance` (acp_serve.go:1760-1798) and `acpDispatcher`'s constructor closure reads `r.patternTable`, `r.hookExec`, `r.learned`, `r.bus` (:596-606). From a foreign package those identifiers are invisible. A func value *can* cross the package boundary; the unexported *name* cannot. Bridge structs therefore take exported fields or exported constructors whose parameters carry the behavior.
**Example sketch (illustrative shape, NOT verbatim code):**

```go
// internal/runtime/enginebridge/adapter.go
type EngineTurnAdapter struct {
    Sess *session.Session
    Mgr  *session.Manager
    Expand   func(*session.Session, []session.ContentBlock) []session.ContentBlock
    Invoke   func([]session.ContentBlock) (key, args string, ok bool)
    AutomationProvenance func() string   // replaces direct r.sessMu read
    // …park fields ride verbatim…
}

// internal/runtime — construction stays in core (runOneTurn today, :961):
adapter := &enginebridge.EngineTurnAdapter{
    Sess: sess, Mgr: sess.Manager,
    Expand: r.expandUserBlocks,   // legal: func value, same package here
    Invoke: r.invocationFor,
}
```

### Pattern 3: Embedded state-owner for the cron family (candidate resolution for OQ 1)

**What:** If the 9 cron methods must physically live in `internal/runtime/cron`, the scheduling *state* they touch (`turnMus`, `turnActive`, `sessMu`, `lastSessionID`, `automationProvenance`, `schedule`, `schedTick`, `schedStop`, `catchUpOnce`) moves into an exported embedded type there; `Runner` embeds it so every call site (`r.startScheduler(ctx)`, `r.sessionTurnMu(id)`) stays verbatim through promotion.
**Constraint discovered:** `acpserve` currently assigns `runner.schedule = scheduleStore` (acp_serve.go:408) — an unexported field of a foreign embedded type cannot be assigned, so the store enters via a constructor (`cron.NewScheduler(store)`) assigned to one exported embedded field. `emitFor` stays on RunnerConfig per D-16, which pulls `startSessionForwarder`'s data dependency toward the core — see Open Question 1 for the two viable shapes.

### Anti-Patterns to Avoid

- **AST/codemod tooling:** the deliverable is a human-reviewable verbatim diff; hand-cut-and-paste with gofmt is the method.
- **Shared testutil package:** D-03 forbids it; duplicate the ≤60-line helpers.
- **Pre-emptive exports "for later phases":** D-19 forbids; export only what fails to compile otherwise.
- **Rewriting while moving:** any "obvious cleanup" breaks the reviewability criterion and the equivalence proof.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Verifying "nothing changed" | Custom behavioral diff harness | Standing `mise ci` gate + test-count ledger + `git diff --color-moved=plain` | `[VERIFIED: .mise.toml [tasks.ci]]` — the gate is the locked equivalence proof (RUNT-01); color-moved makes git itself attest the blocks moved unchanged |
| Import formatting in 20+ touched files | Hand-aligned import blocks | `mise fmt` (gofmt + goimports with `local-prefixes: github.com/Djarvur/ass-guard-agent`) | `[VERIFIED: .mise.toml [tasks.fmt]; .golangci.yml:122-129]` |
| Missing-constant discovery after D-10 split | Manual audit of constant usage | `golangci-lint run` — goconst (enabled via `default: all`) flags repeated string literals per package | `[VERIFIED: .golangci.yml:10 'default: all']` — the linter tells you which constants each new package still needs |
| History tracking through splits | Renaming heuristics | `git mv` for whole-file moves (cron_wiring.go, checkpoint.go); `git log --follow` afterwards | standard git behavior `[ASSUMED]` |

**Key insight:** In a verbatim-refactor phase the tools that matter are the ones that *observe* (gate, diff, ledger), not the ones that *generate*. Every generation step is a pair of hands and an import fix.

## File Inventory (the move targets, verified this session)

### Production code

| Source file | Lines | Destination per CONTEXT | Contents (key decls, line numbers) |
|---|---|---|---|
| `cmd/ass-guard/acp_serve.go` | 2115 | split across runtime / enginebridge / acpserve / cmd | const block :47-66 (`mnd6`, `mcpStartTimeout`, `mutatingCommandCause`, `skillToolName`, `stagePostImplement/Explore/Propose`); `stubExecResult` :71; `nopHost` :76; `stubCatalogExec` :81-98; `toProfileDecls` :86; `serveOptions` :107-120; `redactorAdapter` :123-135; `newACPCmd`/`newACPServeCmd` :139-192 (**stay in cmd**, D-09); `runACPServeCmd` :198-231 (**de-cobra**: reads `audit-log` via `cmd.Flags()`); `startAuditMirror` :237; `resolveWorkDir` :267; `seedACPGuard` :282; `resolveProfilesDir` :298 (**de-cobra**: `cmd.Flags().Changed`); `runACPServe` :320-425 (runner literal :361-382; `acp.NewServer` :397; `runner.emitFor = srv.Emitter` :411; `runner.startScheduler(ctx)` :412; `schedule` assignment :408); `sessionTurnRunner` struct :441-544; methods/setupEngine :549-2095; adapters: `engineTurnRunnerAdapter` :1716, `acpDispatcher` :1949, `patternNextPrompter` :2026, `hookSessionTurnRunner` :2062, `hookSessionBoundaryOpener` :2077, `realCommandRunner` :2092; helpers `mapAskStop` :840, `stopAskACP` :850, `startChunkForwarder` :856, `parkedChain` :1016, `chainIdlePollInterval` :1117, `advisoryNote` :1587, `advisoryNoteText` :1645, `toContentBlocks` :1699, `firstTextBlockIndex` :715, `toolCallNamesUnder` :1929, `triggerFromSignal` :2035, `resolveSubagentModel` :1448, `mergeMCPServers` :1505 |
| `cmd/ass-guard/cron_wiring.go` | 258 | internal/runtime/cron (mechanism: OQ 1) | 9 methods, all on `*sessionTurnRunner`: `sessionTurnMu` :34, `clientTurnActive` :43, `markClientTurn` :53, `currentSessionID` :62, `startScheduler` :73, `fireDueAutomations` :105, `runAutomationTurn` :132, `runCatchUpOnce` :209, `startSessionForwarder` :229; `defaultSchedTick` :26 |
| `cmd/ass-guard/checkpoint.go` | 146 | split: adapter→runtime (D-17), CLI→own pkg (D-08) | `checkpointerAdapter` :22-30; `newCheckpointCmd` :43 (**stays in cmd**, D-09); `runCheckpointList` :101; `runCheckpointRestore` :131; consts :16/:33/:36 |
| `cmd/ass-guard/learning_cmd.go` | 134 | logic→own pkg; cobra stays | `newLearningCmd` :18 (stays); `resolveLearnedPath` :55; `runLearningList` :65; `runLearningRevert` :101 |
| `cmd/ass-guard/modelrouting.go` | 217 | logic→own pkg; cobra stays | `newModelRoutingCmd`/Validate/Resolve :20/:35/:71 (stay); `emitResolveHuman` :134; `emitResolveJSON` :165; `describeCapabilities` :200 |
| `cmd/ass-guard/provider_factory.go` | 171 | own pkg (consumed by acpserve + tracer + parity) | `globalConfigPath` :20; `projectConfigPath` :34; `loadModelRoutingFactory` :50; `setupModelRouting` :98; `setupProviderFactory` :130; `firstDeclaredProvider` :138; `warnLooseConfigPerm` :159 — **note**: called from `main.go` (tracer), `parity.go`, and `runACPServe` (:339) — this is shared infrastructure, one package, imported by three |
| `cmd/ass-guard/parity.go` | 376 | logic→own pkg; cobra stays | seams `zcodeInstalledVersion` :30, `parityRun` :44 (`//nolint:gochecknoglobals` must ride along); `newParityCmd` :46 stays; run logic below |
| `cmd/ass-guard/profile_check.go` | 236 | **NOT covered by D-08 — see Open Question 5** | `newProfileCheckCmd`/`newProfileCmd` :20/:45; `runProfileCheck` :58; capture/report helpers |
| `cmd/ass-guard/goconst_constants.go` | 16 | split by destination (D-10) | verbatim contents: `blockText`, `stopEndTurn`, `protocolVersion20`, `keySessionID`, `keyType`, `tierHeavy`, `tierLight`, `profileZcode`, `implementationCompleteMsg`, `flagProfilesDir`, `actionContinue`, `textListKey`, `chunkDone` `[VERIFIED: goconst_constants.go:1-16, read in full]` |
| `cmd/ass-guard/main.go` | 163 | stays (rewired) | root command + `AddCommand` wiring :77-82; `runTrace` uses `setupProviderFactory` :129 |

### Test-code subject map (D-01..D-04; per-test assignment is planner discretion)

Runner literals per file (grep-verified counts): acp_engine_e2e 3 · acp_serve_test 5 · checkpoint_test 1 · integration 1 · advisory_wiring 1 · planmode_wiring 1 · ask_wiring 2 · mcp_tracer 1 · e2e_opsx 1 · e2e_opsx_matrix 1.

| Test file | Subject evidence | Disposition signal |
|---|---|---|
| `acp_serve_test.go` (1993 ln, 27 tests) | MIXED: `newExpansionRunner`/`newSkillRunner` battery = runner; `TestACPServeWiresStdoutClean` :84, `TestACPServeNoStdoutPollutionFromLogs` :132 call `runACPServe` :99/:142; `TestServeAudit_RequestShapedThroughRealSeam` :1537, `TestServeMirror_Override` :1926, `TestServeTranscriptWriter_OnePerSession` :1412 also `runACPServe`-driven; `TestACPServeCommandRegistered` :35 is cobra-shape | runner tests → runtime (D-01); `runACPServe`-layer tests → acpserve (subject rule, D-02); command-registration → cmd |
| `ask_wiring_test.go` (1144) | runner + 2× `acp.NewServer` :616/:773 | split per D-02 |
| `advisory_wiring_test.go` (449) | runner + `acp.NewServer` :92 | split per D-02 |
| `planmode_wiring_test.go` (366) | runner + `acp.NewServer` :154 | split per D-02 |
| `integration_test.go` (298) | constructs runner :71 + `acp.NewServer` :80 **in-process** (driveACP) — *not* binary-spawn | D-04 names it "stays in cmd", but subject is runner+handler composition — see OQ 3 |
| `e2e_opsx_test.go` (521) / `e2e_opsx_matrix_test.go` (1038) | construct runner in-process (:153 / :146); `exec.CommandContext` targets the *openspec binary*, not ass-guard; define `newOpsxRunnerAt`/`opsxRunnerSeam` consumed by `evalsuite_bridge_test.go` :51/:85/:154 | same nuance as integration — OQ 3 |
| `evalsuite_bridge_test.go` (176) | drives `newOpsxRunnerAt` via `evalharness.RunnerSeam`; gated by `ASSGUARD_EVAL_GATE` | follows the opsx helpers wherever they land |
| `zeroconfig_test.go` (240) | TRUE binary spawn: `go build` :52 + `exec.CommandContext(…, bin, "acp", "serve")` :166 | stays in cmd (D-04's letter and spirit) |
| `acp_engine_e2e_test.go` (473) | runner + defines `scriptedACPProvider`/`scriptedResp` :27/:33 consumed by 7 files | runner subject → runtime; helpers duplicated per D-03 |
| `mcp_tracer_test.go` (388) | `newTracerRunner` :107 constructs runner; **owns `TestMain` :35** (echo-server self-exec mode) | moves with runner; `TestMain` uniqueness constraint (Pitfall 5) |
| `checkpoint_test.go` (451) | MIXED: CLI list/restore tests :56-142 vs `TestSessionFor_WiresCheckpointer` :166 + gated live rollback :313 (constructs runner :358) | CLI tests follow checkpoint CLI pkg; sessionFor tests → runtime (D-17) |
| `coreexec_wiring_test.go` (550) | runner via `newExpansionRunner` (Bash/Todo batteries, runtime-workdir composition) | runner subject → runtime |
| `background_wiring_test.go` (254) | pure coreexec registry — **zero runner refs** | technically independent; follows coreexec subject (likely runtime tests anyway — planner call) |
| `subagent_tier_wiring_test.go` (158) | `tierWiringRunner` :88 chains `newExpansionRunner` + `setupModelRouting` + `writeTestModelRouting`/`pinEmptyHome` (defined in provider_factory_test.go) | runner subject → runtime; cross-package helper duplication required (D-03) |
| `interactive_wiring_test.go` (82) | pure catalog registration, no runner | planner call; small enough to duplicate/stay |
| `learning_cmd_test.go` (126), `modelrouting_test.go` (116), `provider_factory_test.go` (403), `parity_test.go` (382), `profile_check_test.go` (224) | CLI-support subjects, follow their sources | move per D-08 destinations |

Cross-file test-helper graph (duplicated per D-03 at destinations):

- `newExpansionRunner` (acp_serve_test.go:237) ← consumed by advisory, ask, cron, checkpoint, coreexec, subagent_tier, acp_serve tests.
- `scriptedACPProvider`/`scriptedResp` (acp_engine_e2e_test.go:27/:33) ← consumed by 7 files (23 refs in acp_serve_test alone).
- `writeTestModelRouting`/`pinEmptyHome` (provider_factory_test.go:114/:130) ← consumed by subagent_tier tests.
- `newOpsxRunnerAt`/`opsxRunnerSeam` (e2e_opsx_test.go:123/:277) ← consumed by evalsuite_bridge_test.go.

## Runtime State Inventory

Refactor phase — all five categories answered explicitly.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None affected. Transcripts/audit/checkpoint trees under `.ass-guard/` are keyed by sessionID/workdir strings that the move does not rename; no DB stores package paths. Log-line prefixes (`"ass-guard: …"`) are string literals that move verbatim. | None — no data migration exists in this phase |
| Live service config | The operator's Zed (client) config spawns the binary by CLI contract (`ass-guard acp serve` + flags `--profile/--max-concurrent/--profiles-dir/--work-dir/--no-engine/--ask-timeout/--audit-log`). Contract lives outside the repo. Protected by construction: D-09 keeps cobra definitions in cmd. | Verify-by-test: pin the flag/command surface (see Validation Architecture); live Zed session is success criterion #2 |
| OS-registered state | None — no launchd/systemd/Task Scheduler/pm2 registrations in the repo (binary is spawned per-session by the editor). | None |
| Secrets/env vars | Env-gated test skips move WITH their test files: `ASSGUARD_EVAL_GATE`, `ASSGUARD_OPENSPEC_BIN`, `ASSGUARD_E2E_LLM` (e2e_opsx_test.go:51-66), `ASSGUARD_CHECKPOINT_E2E` + `ZAI_API_KEY` (checkpoint_test.go:217-218), `ASSGUARD_E2E_CAPTURE`. Names unchanged — code relocation only. `ZAI_API_KEY` read at provider layer (untouched). | None — env names survive verbatim |
| Build artifacts | None persistent. `go build ./...` output is ephemeral; zeroconfig test rebuilds its binary per run; no installed copies, no egg-info equivalents, no image tags. Stale `~/.cache/go-build` entries are content-addressed (harmless). | None |

**Canonical question answered:** after every file is updated, nothing outside the repo holds the old layout — the only external memory is the Zed spawn contract, which D-09 freezes.

## Common Pitfalls

### Pitfall 1: Methods cannot move to a different package than their receiver (D-13/D-15 collision)
**What goes wrong:** Planning "cron_wiring.go's 9 methods land in `internal/runtime/cron`" verbatim fails to compile — Go requires method receivers' base type to be declared in the same package. Worse, 4 of the 9 are called from runner-core code: `sessionTurnMu` (Run :763; runOneTurn :967), `markClientTurn` (Run :770,:772), `startSessionForwarder` (sessionFor :1389), `runCatchUpOnce` (sessionFor :1395) — a naive move also creates a runtime↔cron import cycle. `[VERIFIED: cron_wiring.go read in full; call sites via grep of acp_serve.go]`
**Why it happens:** D-15's phrasing ("they are methods on the runner today; they land in internal/runtime/cron") encodes intent (physical home) that Go cannot express literally.
**How to avoid:** Pick one mechanism in the plan: (a) **embedded state-owner** — cron pkg owns an exported type holding the 9 scheduling-state fields + the methods; `Runner` embeds it; call sites unchanged via promotion; store injected via `cron.NewScheduler` because acpserve can no longer assign `runner.schedule` directly (:408); `startSessionForwarder`'s `emitFor` dependency argues this method (and possibly `sessionTurnMu`/`markClientTurn`, called from Run) stay in a core `cron.go` *file* with the loop-only methods (`startScheduler`, `fireDueAutomations`, `runAutomationTurn`, `runCatchUpOnce`, `currentSessionID`) in the cron package; or (b) **file-level split only** — all 9 remain methods in package runtime (file `cron.go`), `internal/runtime/cron` receives the loop as free functions taking the narrow state — this bends D-13/D-15 and needs operator sign-off.
**Warning signs:** any plan task saying "move cron_wiring.go methods to cron package" without naming the receiver mechanism.

### Pitfall 2: redactorAdapter's D-07/D-11 tension
**What goes wrong:** D-07 sends `redactorAdapter` to acpserve; but `sessionFor` constructs it twice (acp_serve.go:1157, and the temp-dir fallback :1160: `session.NewManager(dir, sessionID, redactorAdapter{})`) — runtime core cannot reference an acpserve type (acpserve composes runtime; reverse import = cycle).
**How to avoid:** Verbatim-duplicate the 12-line adapter into internal/runtime (consistent with D-03/D-10 duplication discipline), keeping the acpserve copy only if serve-side code uses it — grep shows runner `sessionFor` is its *only* consumer, so the honest reading may be "it belongs to runtime; D-07's mention rides on its textual proximity to serve wiring." Surface in plan as a one-line decision. `[VERIFIED: acp_serve.go:1157-1160 read; adapter def :123-135 read]`

### Pitfall 3: Lint path-exclusions silently stop applying
**What goes wrong:** `.golangci.yml` grants `errcheck` relief on `path: "cmd/.*"` for `(fmt.Println|os.Exit|log.Fatal)` (:112-115). Files leaving cmd lose that shield under `default: all`.
**How to avoid:** After each wave run full `mise lint`; expect new errcheck hits only in moved code that used the relaxed forms (survey shows the CLI files write via `fmt.Fprintf(w, …)` with `_ =` prefixes — low exposure, but parity.go's seams and main-path code deserve a look). Test-scoped relaxations (`.*_test\.go`) travel with the files — those are safe. `[VERIFIED: .golangci.yml read in full]`

### Pitfall 4: nolint directives are line-anchored — reordering regenerates violations
**What goes wrong:** 30+ `//nolint:` comments ride specific lines (`funcorder` ×13, `contextcheck` ×6, `wrapcheck` ×5, `funlen`, `maintidx`, `containedctx`, `cyclop`, `lll`, `staticcheck`, `modernize`, `gocritic`, `forcetypeassert` ×3). Splitting a file redistributes functions; funcorder (declaration-order linter) evaluates each *new file's* ordering, so a function whose old neighbors suppressed its violation may now trip it even with its own nolint intact.
**How to avoid:** Preserve the original relative declaration order within each destination file; keep the existing grouping comments (`//nolint:funcorder // ordering groups related logic`) adjacent to their functions verbatim; treat any new lint firing as a *placement* bug first, never delete the moved code's nolints.
**Warning signs:** `mise lint` failures naming functions that were clean before the move.

### Pitfall 5: TestMain uniqueness
**What goes wrong:** `mcp_tracer_test.go:35` defines the package's only `TestMain` (echo-server self-exec mode). Moving it into `internal/runtime` while any other moved file also declares `TestMain` → compile error; leaving it in cmd while tracer tests move → dead echo-mode code in cmd.
**How to avoid:** Move `mcp_tracer_test.go` as a unit with its `TestMain`; assert exactly one `TestMain` per destination package. `[VERIFIED: grep — single TestMain in cmd]`

### Pitfall 6: Two cobra-typed signatures straddle the D-09 boundary
**What goes wrong:** `runACPServeCmd(ctx, cmd *cobra.Command, …)` (:198) and `resolveProfilesDir(cmd *cobra.Command, …)` (:298) mix flag-reading with serve/support logic. Moving them verbatim drags cobra into acpserve/CLI packages and blurs D-09.
**How to avoid:** Smallest mechanical adaptation: caller (cmd) extracts the plain values (`audit-log` string; `changed bool` + value) and passes them as parameters. Everything else in the move set is cobra-free already. `[VERIFIED: both signatures read]`

### Pitfall 7: Startup-order and emitter-injection invariants
**What goes wrong:** Reordering breaks observable behavior: `srv.Emitter` must exist before `runner.startScheduler(ctx)` (WINDOWS #3 — automation/timer chunks reach the client); `loadCommandRegistry`/`setupEngine` precede server construction; schedule-store failure degrades to a serve *without* scheduled firings (:403-409); ctx-done goroutine reaps MCP hosts (:417-422).
**How to avoid:** acpserve.Run reproduces runACPServe's statement order line-for-line; the review recipe (color-moved diff) makes any reorder visually obvious. `[VERIFIED: acp_serve.go:320-425 read]`

### Pitfall 8: Constants and helpers vanish across the split
**What goes wrong:** After D-10's per-package split, a moved function referencing `blockText`/`stopEndTurn`/etc. fails to compile or — worse — a raw literal reintroduced silently diverges. Usage spans nearly every file (46 refs in acp_serve_test.go, 42 in ask_wiring_test.go, 30 in acp_engine_e2e_test.go…).
**How to avoid:** Copy the full constant set into each destination that uses ≥1 of them (precedent: `internal/acp/goconst_constants.go` already duplicates `blockText`, `stopEndTurn`, `protocolVersion20`, `keySessionID` verbatim — the pattern is established in-repo `[VERIFIED: internal/acp/goconst_constants.go read in full]`); let goconst lint catch misses.

### Pitfall 9: D-04's file names don't match its mechanism description
**What goes wrong:** D-04 keeps "true stdio E2E suites (spawn binary + JSON-RPC): e2e_opsx*, integration" in cmd — but the verified mechanism shows `e2e_opsx_test.go`, `e2e_opsx_matrix_test.go`, and `integration_test.go` construct the runner **in-process** (only `zeroconfig_test.go` and mcp_tracer's echo mode spawn binaries; e2e_opsx's exec calls target the *openspec* CLI). Following D-04's letter strands runner-construction tests in cmd where the type no longer exists.
**How to avoid:** Apply D-02's subject rule to these three files (runner subject → runtime) and reserve D-04 for the genuinely binary-spawning suites (zeroconfig; tracer echo). This sits squarely in CONTEXT's "Claude's Discretion: exact per-test assignment," but the deviation from D-04's letter deserves explicit notation in the plan. See OQ 3.

### Pitfall 10: Compile-closure illusion in wave planning
**What goes wrong:** Splitting the carve into "Wave 1: move runner; Wave 2: rewire cmd" produces a permanently-red tree between waves — cmd references `sessionTurnRunner`, `runACPServe`, `serveOptions`, and 21 test literals that all move together. There is no intermediate compiling state short of shims (which violate verbatim reviewability).
**How to avoid:** Define the carve wave as one compile closure with ordered tasks whose *progress metric* is `go build ./internal/...` + targeted package tests, with `mise ci` green required only at the wave boundary. CLI-support extraction (wave 2) genuinely compiles independently per file.

## Code Examples

### The composition acpserve must produce (source-verbatim today, acp_serve.go:361-412)

```go
// Source: cmd/ass-guard/acp_serve.go (read this session) — the exact order
// acpserve.Run must reproduce; runner literal becomes RunnerConfig+NewRunner (D-05).
runner := &sessionTurnRunner{
    bus: bus, bodyStore: bodyStore, profile: prof,
    workDir: opts.WorkDir, maxConc: opts.MaxConcurrent,
    configAdded: opts.ConfigAddedBoundaries, askTimeout: opts.AskTimeout,
    serveCtx: ctx, schedCfg: schedCfg, providerName: providerName, stderr: stderr,
    makeProvider: func(capturer provider.RequestCapturer) provider.Provider {
        p, _ := factory.BuildWithCapturer(providerName, shaper.New(), capturer)
        return p
    },
}
runner.loadCommandRegistry()
if opts.EngineEnabled { /* degrade, never crash */ }
srv := acp.NewServer(in, out, stderr, acp.WithTurnRunner(runner))
// …schedule-store open, degrade loudly on failure…
runner.emitFor = srv.Emitter // WINDOWS #3 — must stay AFTER NewServer, BEFORE startScheduler
runner.startScheduler(ctx)
```

### The interface seam that makes the carve safe (unchanged)

```go
// Source: internal/acp/server.go:31-42 (read this session)
type TurnRunner interface {
    Run(ctx context.Context, sessionID string, emit ChunkEmitter, prompt []ContentBlock) (stopReason string, err error)
}
type SessionCloser interface {
    CloseSession(sessionID string) error
}
```

`runtime.Runner` implements both exactly as `sessionTurnRunner` does today (`Run` :753, `CloseSession` :1564) — `acp.WithTurnRunner(runner)` plugs back identically.

### Review recipe for the pure-relocation criterion

```bash
git diff --color-moved=plain --color-moved-ws=allow-indentation-change master -- cmd/ internal/
# Blocks shown as moved-not-edited attest verbatim relocation; any block
# rendered as changed is a review flag.
```

## State of the Art

Not applicable — this is a timeless structural refactor of a Go 1.26 codebase with a pinned toolchain. One relevant evolution note: golangci-lint v2 replaced `wsl`/`gomodguard` (disabled in config accordingly) `[VERIFIED: .golangci.yml:11-13]`; nothing in the phase interacts with deprecated features.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Methods must be declared in the same package as their receiver's base type (basis of Pitfall 1 / OQ 1) | Common Pitfalls | Low — Go-spec bedrock `[CITED: go.dev/ref/spec#Method_declarations]`; but if the planner doubts it, a 5-minute scratch compile settles it before planning around it |
| A2 | `funcorder` evaluates declaration order per-file, so redistribution can create new findings despite surviving nolints | Pitfall 4 | Low-Medium — worst case a few lint iterations; mitigation is order preservation |
| A3 | Zed/operator config spawns `ass-guard acp serve` with today's flags; no other automation registers the binary | Runtime State Inventory | Medium if wrong (silent operator breakage) — mitigated by D-09 + recommended flag-surface parity test |
| A4 | `git --color-moved` reliably renders the moved blocks for review | Don't Hand-Roll / review recipe | Low — cosmetic review aid; gate + ledger carry the proof regardless |
| A5 | goconst fires on repeated literals in the new packages under `default: all` with default thresholds (min-occurrences 3) | Don't Hand-Roll / Pitfall 8 | Low — if threshold differs, misses surface as compile errors instead (safe direction) |

## Open Questions

1. **Cron-family mechanism (resolves Pitfall 1)** — embedded state-owner type in `internal/runtime/cron` with promoted methods (preserves every call site verbatim; requires `cron.NewScheduler(store)` injection because acpserve's `runner.schedule = …` assignment dies), versus core-file retention for the four core-called methods + cron-package home for the loop. What we know: exact call-site lines verified; both shapes keep behavior identical. What's unclear: which shape the operator prefers under D-13/D-15's "land in internal/runtime/cron". Recommendation: option (a) — embedded `cron.Scheduler` owning loop-only methods (`startScheduler`, `fireDueAutomations`, `runAutomationTurn`, `runCatchUpOnce`, `currentSessionID`) with `sessionTurnMu`/`markClientTurn`/`clientTurnActive`/`startSessionForwarder` as core methods (their callers and `emitFor` dependency live in core); planner documents the deviation from D-13's letter in one sentence.
2. **redactorAdapter home (Pitfall 2)** — runtime-only (its sole consumer is sessionFor) vs duplicated runtime+acpserve per D-07's letter. Recommendation: runtime-only, noted in plan; zero behavior impact either way.
3. **True home of e2e_opsx*/integration tests (Pitfall 9)** — D-04's letter (stay in cmd) vs their verified in-process runner subject (→ runtime, per D-02 subject rule within Claude's Discretion). Recommendation: follow the subject; keep only zeroconfig (+ tracer echo mode) in cmd as the binary-spawn suites.
4. **enginebridge surface details** — exported fields vs exported constructors on the 7 adapters; fate of `patternNextPrompter` (assertion target must be visible to its asserter). Planner discretion within D-12/D-14; constructors recommended (keeps zero-value structs unusable-by-accident).
5. **profile_check.go disposition** — absent from D-08's enumerated list though it is a cmd non-test file like the other five (its `newProfileCmd` wires into root via main.go:77). Recommendation: same treatment — own package per D-08's pattern — flagged here because CONTEXT did not lock it; one-line user confirmation is cheap insurance.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | everything | ✓ | 1.26.1 (mise-pinned "1.26") | — |
| golangci-lint | lint gate | ✓ | 2.12.2 (v2 config) | — |
| mise | task runner (ci/vet/lint/build/test/fmt) | ✓ | 2026.8.12 | direct commands |
| CGO_ENABLED=0 build | static-binary guarantee | ✓ | vet+build passed this session | — |
| git | mv/history/color-moved review | ✓ | system | — |
| ZAI_API_KEY | gated live legs (eval/checkpoint-e2e) | not required | — | skipped by env gates; not part of `mise ci` |
| openspec binary | gated e2e legs | not required | — | skipped outside ASSGUARD_OPENSPEC_BIN=1 |

**Missing dependencies with no fallback:** none.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard testing + `-race`; testify in some suites; orchestrated via mise |
| Config file | `.mise.toml` (`[tasks.ci]` = vet → lint → build → test); no test-framework config file beyond go.mod |
| Quick run command | `go build ./... && go vet ./... && go test ./internal/runtime/... ./internal/acpserve/... ./cmd/ass-guard/ -count=1` (<60s target) |
| Full suite command | `mise ci` (vet + `golangci-lint run` + `CGO_ENABLED=0 go build ./...` + `go test -race -count=1 ./...`) |

### Phase Requirements → Test Map

RUNT-01's proof is *equivalence*: the existing suite passes unchanged from new homes. No new feature tests; two cheap guards recommended.

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| RUNT-01 | All 130 existing test functions still pass, relocated | regression (existing suite) | `mise ci` | ✅ (baseline: 130 `^func Test` in cmd today — ledger captured pre-move) |
| RUNT-01 | Relocated tests execute in their new packages | structural | `go test ./internal/runtime/... ./internal/acpserve/... -count=1 -v \| grep -c '^=== RUN'` compared to ledger | ❌ Wave 0 ledger artifact |
| RUNT-01 | CLI contract unchanged (commands + flags byte-identical) | contract smoke | golden-list test asserting root/acp/checkpoint/learning/model-routing/profile/parity command+flag sets | ❌ Wave 0 recommended addition (~80 lines; protects success criterion #2 mechanically) |
| RUNT-01 | Binary handshake still clean (stdout = ACP frames only) | e2e smoke (exists) | `go test ./cmd/ass-guard/ -run TestZeroConfigFirstRun -count=1` (builds binary, spawns `acp serve`, no keys needed) | ✅ zeroconfig_test.go |
| RUNT-01 | Diff is pure relocation | review aid (manual) | `git diff --color-moved=plain master` + moved-line ratio in PR description | manual-only — justified: reviewer judgment is the criterion |
| RUNT-01 | Live Zed session identical (criterion #2) | manual-only | operator's daily-use session | manual-only — justified: requires editor + live credentials; automated coverage above (handshake smoke + full race suite) bounds the risk |

### Sampling Rate

- **Per task commit:** quick run command (build + vet + touched-package tests).
- **Per wave merge:** `mise ci` green — the RUNT-01 equivalence proof at each boundary.
- **Phase gate:** full `mise ci` + test-count ledger match + color-moved diff review + live-Zed checklist handed to operator before `/gsd:verify-work`.

### Wave 0 Gaps

- [ ] Test-count ledger (pre-move): `grep -h "^func Test" cmd/ass-guard/*_test.go \| wc -l` (=130 today) recorded in the plan; post-move sum across cmd + new packages must equal it.
- [ ] CLI-contract golden test (recommended above) — the only suggested *new* test; everything else reuses the standing suite.
- [ ] Framework install: none needed.

## Security Domain

`security_enforcement` is not disabled in `.planning/config.json` (key absent → enabled). This is a behavior-preserving refactor, so the security posture is inherited; the phase's duty is *not weakening* it.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | provider credentials handled at internal/provider (untouched); API keys env/file only (project decision) |
| V3 Session Management | no | ACP session lifecycle untouched in internal/acp |
| V4 Access Control | no | permission/gate pipeline arrives in Phase 17; nothing added here |
| V5 Input Validation | inherits | flag parsing stays in cmd (D-09); config loading paths move verbatim (`loadModelRoutingFactory` stat-gating) |
| V6 Cryptography | no | none in scope |
| V14 Config Hygiene | yes | `warnLooseConfigPerm` (0600 advice) moves verbatim — must not be dropped in the providerfactorycmd split `[VERIFIED: provider_factory.go:159-171 read]` |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| CLI contract drift breaking the editor spawn (or widening it) | Tampering / Elevation | D-09 (cobra stays in cmd) + flag-surface golden test |
| Transport-discipline breach (diagnostics leaking to stdout → corrupt ACP stream) | Information Disclosure | existing stderr discipline preserved verbatim; `TestACPServeWiresStdoutClean`/`NoStdoutPollutionFromLogs` relocate with acpserve and keep guarding |
| Credential-on-disk exposure regression | Information Disclosure | `warnLooseConfigPerm` verbatim move + its existing tests (`TestStartupWarn_ConfigPermLoose`) following it |
| Audit-trail degradation during split (mirror/bodyStore dropped) | Repudiation | `startAuditMirror`/bodyStore construction order pinned in acpserve.Run; `TestServeAudit_*`/`TestServeMirror_Override` cover it |

## Sources

### Primary (HIGH confidence — direct reads this session)
- `cmd/ass-guard/acp_serve.go` — lines 1-120, 120-450, 449-559, 1120-1250, 1240-1410, 1699-1828 read in full; remainder inventoried via declaration grep (line-numbered listing above)
- `cmd/ass-guard/cron_wiring.go`, `checkpoint.go`, `main.go`, `learning_cmd.go`, `modelrouting.go`, `provider_factory.go`, `profile_check.go`, `goconst_constants.go` — read in full; `parity.go` lines 1-60
- `.golangci.yml`, `.mise.toml`, `.planning/config.json`, `.planning/ROADMAP.md` (Phase 15 §), `REQUIREMENTS.md`, `STATE.md`, `15-CONTEXT.md`
- `internal/acp/server.go` (interfaces), `internal/acp/goconst_constants.go` (duplication precedent)
- Test-file subjects verified via targeted greps + reads (acp_serve_test, subagent_tier_wiring, mcp_tracer TestMain, interactive_wiring, checkpoint_test, evalsuite_bridge, zeroconfig, e2e_opsx headers)

### Secondary (MEDIUM confidence)
- Go spec, method declarations — `go.dev/ref/spec#Method_declarations` `[CITED, not fetched this session]`

### Tertiary (LOW confidence)
- None — no web research performed (per ROADMAP: "Standard patterns (skip research-phase): Phase 15").

## Metadata

**Confidence breakdown:**
- Move-target inventory & test-subject map: HIGH — every file opened or line-grepped this session; counts quoted from grep output
- Structural constraints (method/package rule, adapter visibility, compile closure): HIGH — grounded in read source + Go semantics; the two decision points (OQ 1/2) are surfaced, not hidden
- Lint/gate behavior: HIGH — config read in full; tool versions probed live
- Pitfall mechanics (A2/A5): MEDIUM — flagged in Assumptions Log

**Research date:** 2026-08-26
**Valid until:** 2026-09-25 (stable — codebase-grounded; re-validate if acp_serve.go churns)

## RESEARCH COMPLETE

**Phase:** 15 - internal/runtime Carve (Step 0)
**Confidence:** HIGH

The carve is fully inventoried (every destination, every adapter construction site, every test-file subject), the gate strategy is already proven (`mise ci` + ledger + color-moved diff), and the two places where CONTEXT's decisions meet Go's visibility rules — cron methods crossing packages (OQ 1) and redactorAdapter's home (OQ 2) — are documented with concrete resolution options so the planner can lock them in PLAN.md instead of the executor discovering them mid-wave. Three smaller judgment calls (e2e_opsx/integration test homes, enginebridge constructor surface, profile_check.go disposition) are teed up with recommendations.
