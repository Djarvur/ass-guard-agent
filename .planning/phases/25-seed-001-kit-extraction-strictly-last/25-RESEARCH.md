# Phase 25: SEED-001 Kit Extraction (strictly last) - Research

**Researched:** 2026-08-28
**Domain:** Go single-module library extraction — package re-homing (`internal/*` → `kit/*`), seam inversion (kit→app dependency edges), composition-root API, frontend seam (Emitter + Requester) design
**Confidence:** HIGH (codebase-verified import graph + all carried decisions read this session; tooling claims cited from official docs)

## Summary

Phase 25 is a *re-homing + seam-inversion* phase, not a rewrite. The codebase audit (run this session via `go list -f '{{.ImportPath}}|{{join .Imports ","}}' ./internal/... ./cmd/...`) shows the KIT-01 promotion set is almost clean: of the 15 packages to promote (14 KIT-01 packages + `internal/runtime` per D-05), only three packages have edges into app-only code, totaling **eight distinct kit→app dependency edges**: `runtime→acp`, `runtime→ecosys`, `runtime→openspec`, `runtime→coreexec`, `runtime→learning`, `runtime→sched`, `session→ecosys`, `enginebridge→learning`. Every edge has a named, already-decided resolution path in the carried decisions: D-14 (Emitter), D-15 (Requester), D-16 (Scheduler port), D-17 (catalog injection), D-05's per-case justification (coreexec/learning/openspec). The `enginebridge→learning` edge is a single method call (`d.learned.Lookup(situation)`, enginebridge.go:423) — trivially port-shaped.

Two findings materially shape the plan. **First**, the frontend seam is mostly a *formalization* of existing wires, not new machinery: the kit-side event bus already carries neutral event types (`event.AgentMessageChunk`, `event.ToolCall`, `event.ToolCallUpdate` — verified events.go:46-76); the runtime forwarders translate them into `acp` frames *inside the kit today* (`toolKindFor`, `forwardToolCall` in runtime.go:651-739). KIT-02's design act is to stop that translation at the kit boundary: the kit emits its own event vocabulary through one `Emit(ctx, Event)` method, and a new app-side adapter does the frame/stop-reason/kind mapping that today lives in runtime.go. The ask machinery is likewise already kit-side (`session.AskBroker`); the Requester seam formalizes the ctx-blocking ask surface that Phase 17's locked plans (`acpserve/ask_surface.go`, 17-02) already implement acp-side. **Second**, the "strictly last" reality cuts both ways: Phases 17–24 are PLANNED NOT executed, and their locked plans add ~9 new files inside kit-bound packages (`runtime/commands.go`, `runtime/rescan.go`, `session/steerqueue.go`, `session/gate.go`, `modelrouting/outcomes.go`, …) plus 3 new app packages (`perm`, `tasks`, `sandbox`) — the pass-1 move inventory must be taken at execution time, but every seam contract this phase designs is already stable in those locked plans.

The verification story inherits Phase 15's proven instruments (test-count ledger, CLI-contract golden, `mise ci` equivalence) and extends them per D-20 with the behavioral eval suites. One silent-gate hazard found: `scripts/eval-change-class.sh` pins the `turn-behavior` change class to `internal/engine`, `internal/session`, `internal/coreexec` paths (verified eval-change-class.sh:27-40) — after extraction these classes must name `kit/…` or the eval gate stops firing exactly when the riskiest moves land.

**Primary recommendation:** Execute as D-03's two passes — pass 1 moves all 15 packages leaf-first (rank-ordered: event/redact/checkpoint/profile → audit/hookdag/shaper/toolcat/mcp → provider/toolexec/modelrouting → session → engine → enginebridge → runtime) as pure verbatim relocations with `mise ci` green at each step; pass 2 resolves the eight edges into the five kit-defined seams (Emitter, Requester, Scheduler port, CommandCatalog injection, SessionToolkit injection), lands the D-19 import-direction gate only after the last edge is gone, and adds the non-ACP host-proof test as KIT-02's automated criterion.

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Module Boundary & Layout
- **D-01:** Single Go module — the kit is a top-level `kit/` directory inside the existing go.mod. No separate library module (atomic cross-cutting commits preserved; mise/goreleaser untouched; a module split can come later if external release demands it).
- **D-02:** Directory is named `kit/`, NOT `pkg/` (operator free-text: "no pkg but kit, and kit/internal"). Kit-private packages live under `kit/internal/`. This supersedes the letter of KIT-01's "pkg/" — the requirement's intent (importable as a library, outside app-internal visibility) is honored by the kit/ tree.
- **D-03:** Two-pass migration discipline: pass 1 = verbatim re-home (pure moves, `git diff --color-moved` reviewable, `mise ci` as equivalence proof — the Phase-15 discipline); pass 2 = KIT-02 seam + cron port + composition surface design on settled code. Equivalence and design never mix in one diff.
- **D-04:** `kit/internal/` seeded organically — only what extraction forces (likely the Phase-15 enginebridge adapters and shared unexported seams). No pre-designated set; grows as forced, never preemptively public.
- **D-05:** Promotion scope: strict KIT-01 list (profile, shaper, provider, modelrouting, toolcat, toolexec, engine, hookdag, event, session, redact, checkpoint, audit, mcp) + `internal/runtime` (the Phase-15 carve whose package doc already declares kit ambition). Everything else stays app-side unless a kit interface genuinely requires it, planner justifying per-case.
- **D-06:** After extraction, `internal/` = app-only (acpserve, providerfactory, checkpointcmd, learningcmd, modelroutingcmd, profilecheckcmd, paritycli, acp, ecosys, openspec, coreexec, learning, firstrun, evalsuite, parity). `kit/` vs `internal/` is the top-level split; no app/ tree, no moving app packages under cmd/.
- **D-07:** Module path unchanged (`github.com/…/ass-guard`); kit imports read `…/ass-guard/kit/<pkg>`.
- **D-08:** SEED-002 fantasy + SEED-003 landscape stay in `.planning/seeds/` as KIT-03 reading material — canonical-ref'd here, zero runtime deps trivially satisfied.

#### Composition-Root API Shape
- **D-09:** App composes; kit = parts. Every kit package exports its constructors + seam interfaces; `acpserve.Run` stays THE composition root. No `kit.Compose`/`kit.New` mega-assembly.
- **D-10:** Assembly config = flat struct with mirrored names (Phase-15 D-05 precedent): field names mirror today's RunnerConfig (Bus, Profile, WorkDir, MakeProvider, …). No nested per-subsystem blocks, no functional options.
- **D-11:** Thin constructor, explicit steps (Phase-15 D-05 carried): constructors fill structs; the caller invokes startup steps explicitly (loadCommandRegistry / setupEngine / startScheduler ordering). No Start/Stop handle, no framework `Run(ctx)` — the app owns context/cancel per ACP transport discipline.
- **D-12:** Export policy: minimal, demand-grown (Phase-15 D-19 carried). Export only what the reference app + KIT-02 seam + cross-package kit internals need; the public API grows when a real consumer (Telegram, v1.3) forces it.

#### Frontend Seam (KIT-02 — the new design act)
- **D-13:** Seam breadth: minimal pair — **Emitter** (kit events streaming out) + **Requester** (asks coming in: permissions/elicitation). Exactly KIT-02's letter. Session lifecycle, command advertisement, config options stay app-side surfaces.
- **D-14:** Emitter granularity: kit-neutral event vocabulary (session-update, tool-call, plan-delta, …) + a single `Emit(ctx, Event)` method. The acp adapter translates kit events → ACP wire frames. Phase-15 D-20's "no ACP words in runtime API" holds by construction; a Telegram adapter would translate the same events. No typed method-set interface (freezes wide API), no opaque pass-through (proves nothing).
- **D-15:** Requester contract: ctx-blocking `Request(ctx, Ask) Answer`; timeout policy is kit-side config mirroring Phase-12 D-01 (wait interval then non-answer capture-shaped result). Matches today's suspended-turn ask semantics.
- **D-16:** Cron seam: interface port now. The Phase-15 D-13 amendment's deferred design work lands here: a Scheduler port defined on the kit side, cron wiring implements it, runner core depends on the interface — the bidirectional core↔cron calls (4 of 9 cron methods called from core; runAutomationTurn → runOneTurn) become interface calls. No second deferral.

#### App-vs-Kit Boundary
- **D-17:** Ecosystem blindness: the kit consumes an injected command/skill/plugin catalog (interface defined kit-side, mirroring the loadCommandRegistry seam). `.claude/` discovery, precedence merging, OpenSpec hosting stay app-side (ecosys/openspec per KIT-03). SEED-001's "invert .claude/ layout" is satisfied by injection, not promotion.
- **D-18:** SEED-001's app invariants (stdout discipline, read-only dirs) express kit-side by construction: kit APIs take io.Writer / interfaces; no os.Stdout, no hardcoded paths, no `.claude/` assumptions anywhere in kit/. No policy-toggle structs — the app passes what it wants enforced.
- **D-19:** Import direction CI-enforced: kit/ must import nothing from internal/ or cmd/ — mechanical gate in `mise ci` (grep/depguard-style), fails the build. Direction app→kit only; enforced, not conventional.
- **D-20:** Equivalence bar: the full gate family — `mise ci` green + relocated-test ledger match + CLI-contract golden (Phase-15 wave-0 instruments reused) PLUS the behavioral eval suites (change-class gate) — criterion #3's "all behavioral eval suites still pass against the extracted layout" verbatim.

### Claude's Discretion
- Exact kit event vocabulary enum members (session-update, tool-call, plan-delta are directional placeholders, not locked names).
- Scheduler port method shape (driven by the 9 cron methods' actual call graph at planning time).
- Which Phase-15 enginebridge/seam artifacts land in kit/internal vs a public sub-package (organic per D-04).
- Physical file layout within kit packages (mirrors internal/ naming unless a collision forces otherwise).

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope.

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| KIT-01 | Core agent machinery extracted behind a composition-root API in `pkg/` (superseded to `kit/` per D-02): profile/shaper/provider/modelrouting/toolcat/toolexec/engine/hookdag/event/session/redact/checkpoint/audit/mcp | Full inventory verified: all 14 packages + `internal/runtime` located, import graph computed, all 8 kit→app edges enumerated with resolution paths (§Current-State Inventory, §Seam Designs); move order computed from import ranks (§Move Order) |
| KIT-02 | Frontend seam expressed as kit interfaces — emitter + requester defined session-side, implemented acp-side | Seam designs derived from existing wires: kit bus events are already neutral (events.go:46-76); runtime's acp-frame translation identified as the code that moves acp-side; AskBroker is already kit-side; Phase 17's locked plan already lands the acp-side ask surface (§Emitter Seam, §Requester Seam) |
| KIT-03 | ass-guard becomes the kit's reference app — retains ecosys/openspec/coreexec/learning/firstrun/evalsuite/parity + ACP frontend; SEED-002/003 as reading material | Reference-app retention verified: app-side package list (D-06) plus 6 unmapped packages dispositioned; equivalence gate family mapped (mise ci + ledger + CLI golden + eval suites + eval-change-class path extension) (§Validation Architecture, §Reference-App Disposition) |

</phase_requirements>

## Architectural Responsibility Map

This phase's "tiers" are repo tiers: the **kit library** (`kit/…`, public), the **reference app internals** (`internal/…`, app-only), and the **CLI shell** (`cmd/…`).

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Turn loop / session lifetime (Runner, sessionFor, park, cron firing) | kit (`kit/runtime`) | — | D-05 promotion; the SEED-001 kit core (doc.go:9-18 declares it) |
| Provider/shaper/profile/modelrouting/toolcat/toolexec/engine/hookdag/event/session/redact/checkpoint/audit/mcp | kit | — | KIT-01 letter; all 14 verified import-clean of app code except session→ecosys (one edge) |
| Emitter translation (kit events → ACP frames, tool-kind mapping, stop-reason mapping) | app (`internal/acpserve` adapter, or `internal/acp`) | kit (emits raw events) | D-14: "the acp adapter translates kit events → ACP wire frames"; today this code sits inside runtime.go and moves out in pass 2 |
| Requester implementation (permission/elicitation wire asks) | app (`internal/acpserve/ask_surface.go` — locked 17-02 contract) | kit (defines `Requester`, owns AskBroker + timeout) | D-15 contract; acp implements |
| `.claude/` discovery, precedence merging, OpenSpec hosting | app (`internal/ecosys`, `internal/openspec`) | kit (consumes injected catalog) | D-17 ecosystem blindness |
| Core tool executors (Bash/Read/Write/Edit/Todo/Ask/Interactive registration) | app (`internal/coreexec`) | kit (SessionToolkit injection seam) | KIT-03 retains coreexec app-side; D-05 per-case seam |
| Learning store (`.ass-guard/learned.yaml`) | app (`internal/learning`) | kit (one-method `Lookup` seam) | KIT-03 letter; enginebridge's only use is `learned.Lookup(situation)` |
| Cron schedule store (`.ass-guard/schedule/`) | app (`internal/sched`) | kit (Scheduler port: Now/Due/ClaimForFire/CatchUp) | D-16 port; store stays app-side |
| ACP wire server, config surface, replay, permissions wire | app (`internal/acp`, `internal/acpserve`) | — | D-06; Phases 16-18 own these shapes — untouched |
| CLI composition (cobra, flags, subcommands) | cmd (`cmd/ass-guard`) | — | Phase-15 D-09; unchanged |
| Behavioral eval suites, parity/drift, evalharness | app (`internal/evalsuite`, `evalharness`, `parity`, `drift`, `paritycli`, `loop`) | — | D-06; they consume kit via app→kit imports (already the direction) |

## Standard Stack

**No new packages are installed in this phase.** The stack is the existing toolchain, all verified present:

### Core
| Tool | Version (verified this machine) | Purpose | Why Standard |
|------|--------------------------------|---------|--------------|
| Go | go1.26.5 (go.mod: `go 1.26`, module `github.com/Djarvur/ass-guard-agent` — go.mod:1,14) | Compile + `go list` import-graph analysis + import rewrites | The only runtime; single module per D-01 |
| mise | 2026.8.14 | Task gate (`mise ci` = vet+lint+build+test; `eval-gate`, `eval-check-changed`) | The project's equivalence-proof gate (D-20) |
| golangci-lint | 2.12.2 (v2 config) | depguard import-direction rule (D-19) + existing strict linters | Already configured: `.golangci.yml:16-22` has `depguard.rules.Main` (`list-mode: lax`, deny `unsafe`) — the D-19 rule joins it |
| git | 2.50.1 | `git diff --color-moved` pure-move review (D-03) | Phase-15 discipline; zebra/dimmed-zebra modes are the block-safe choices [CITED: git-scm.com/docs/git-diff] |
| `go list -deps` / `goimports` | stdlib tooling | Import-graph audit (this research's method) and mechanical import-path rewrites | No library needed — the compiler is the checker |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| grep-based import gate in mise | depguard-only | depguard is config-native but test-file scoping needs `files:` globs; the grep gate is trivially auditable. Use both (depguard = lint-layer belt, grep = ci-layer suspenders) — D-19 says "grep/depguard-style" |
| `--color-moved=plain` (Phase 15's choice) | `--color-moved=dimmed-zebra --color-moved-ws=allow-indentation-change` | git docs: plain "is not very useful in a review to determine if a block of code was moved without permutation"; zebra-family detects contiguous blocks ≥20 alphanumerics [CITED: git-scm.com/docs/git-diff] — at tree scale zebra-family is the safer reviewer aid |

**Installation:** none.

**Package Legitimacy Audit:** Not applicable — this phase installs zero external packages (pure in-repo re-homing). The module's existing dependencies (anthropic-sdk-go v1.63.0, go-openai v1.42.0, mcp go-sdk v1.7.0, cobra, yaml, html-to-markdown, x/net — go.mod:7-15) are untouched. `kit/` packages keep the same external deps they have today; `go mod tidy` after the moves is the only module-file action (22-04/21-05/22-05 plans already touch go.mod for their own deps, landing before this phase).

## Current-State Inventory (the extraction surface, verified this session)

### Promotion set and import ranks

Computed from `go list -f '{{.ImportPath}}|{{join .Imports ","}}' ./internal/... ./cmd/...`. Rank = longest chain of kit-internal deps; app-only imports marked with (*).

| Rank | Package | Kit-internal deps | App-code deps (edges to resolve) |
|------|---------|-------------------|----------------------------------|
| 0 | `internal/event` | — | none |
| 0 | `internal/redact` | — | none |
| 0 | `internal/checkpoint` | — | none |
| 0 | `internal/profile` | — | none |
| 1 | `internal/audit` | event, redact | none |
| 1 | `internal/hookdag` | event | none |
| 1 | `internal/shaper` | profile | none |
| 1 | `internal/toolcat` | profile | none |
| 2 | `internal/provider` | profile, redact, shaper | none |
| 2 | `internal/mcp` | toolcat | none |
| 3 | `internal/toolexec` | provider, toolcat | none |
| 3 | `internal/modelrouting` | event, profile, provider, shaper | none |
| 4 | `internal/session` | audit, event, profile, provider, toolcat, toolexec | **ecosys** (types only) |
| 5 | `internal/engine` | event, session | none |
| 6 | `internal/runtime/enginebridge` | engine, event, hookdag, session | **learning** (one call) |
| 7 | `internal/runtime` | all of the above | **acp, coreexec, ecosys, learning, openspec, sched** |

Verbatim quotes for the two session-package edges [VERIFIED: internal/session/session.go:63,70]:

```go
	SubagentTypes map[string]ecosys.Agent
	...
	Hooks *ecosys.HookRunner
```

### The eight kit→app edges (what pass 2 resolves)

1. **`runtime → acp`** (the KIT-02 target). runtime.go imports `internal/acp` for: the `Run` signature (`Run(ctx, sessionID, emit acp.ChunkEmitter, prompt []acp.ContentBlock)`, runtime.go:479-482), the forwarders (`routeBusEvent`, `forwardToolCall` building `acp.ToolCallFrame`, `forwardToolCallUpdate` decoding `acp.ToolCallUpdateFrame`, runtime.go:651-703), the presentation mapping `toolKindFor` → `acp.ToolKind*` constants (runtime.go:707-739), `SetEmitter(emit func(sessionID string) acp.ChunkEmitter)` (runtime.go:1502), `emitFor` field (runtime.go:190), and `toContentBlocks` (acp blocks → session blocks, runtime.go:1632-1639). The acp interfaces today [VERIFIED: internal/acp/server.go:20-33,40-42]:

```go
type ChunkEmitter interface {
	// AgentMessageChunk streams one text chunk as a session/update notification.
	AgentMessageChunk(messageID, text string) error
}
type TurnRunner interface {
	Run(ctx context.Context, sessionID string, emit ChunkEmitter, prompt []ContentBlock) (stopReason string, err error)
}
type SessionCloser interface {
	CloseSession(sessionID string) error
}
```

2. **`runtime → ecosys`** (D-17 target). Uses: `ecosys.Discover` + `Registry`/`ServerConfig` fields (runtime.go:162-169,357-373), `ecosys.ParseInvocation` (runtime.go:403,466), `openspec.MutabilityMutating` comparison (runtime.go:423), `ecosys.SkillListing`/`AgentListing` profile merges (runtime.go:1056,1072), `ecosys.SkillExecute` Skill-tool override (runtime.go:1092), `ecosys.NewHookRunner` (runtime.go:1106), `r.reg.Agents` → `SubagentTypes` (runtime.go:1190), `mergeMCPServers(project mcp.Config, lower []ecosys.ServerConfig)` (runtime.go:1368).

3. **`runtime → openspec`** (D-17 target). `SetupEngine` loads `openspec.DefaultConfig()`, `openspec.RegisterTools(catalog, oscfg)`, `openspec.FromConfig(oscfg)` → pattern table (runtime.go:275-297); `LoadCommandRegistry` reads `oscfg.CommandMutability` (runtime.go:375-382).

4. **`runtime → coreexec`** (the big seam). `sessionFor` builds the per-session toolkit: `coreexec.NewTaskRegistry`, `coreexec.RegisterCore(sCatalog, coreexec.Config{WorkDir, Todos, Hooks, Tasks})`, `coreexec.RenderAskSurface(p.Questions)` as the ask surface callback, `coreexec.RegisterAsk(sCatalog, askBroker)`, `coreexec.NewAgentMailbox`, `coreexec.NewSessionReader`, `coreexec.RegisterInteractive(sCatalog, coreexec.InteractiveConfig{Ask, PlanMode, Mailbox, Sessions, Tasks, Schedule})` (runtime.go:1107-1140); `taskRegistry.ReapAll()` in OnClose (runtime.go:1240).

5. **`runtime → learning`**. `learning.Open(learnedPath)` → `*learning.Store` in SetupEngine (runtime.go:308-315); store passed into `enginebridge.BridgeConfig.Learned`.

6. **`runtime → sched`**. `SetSchedule(store *sched.ScheduleStore)` (runtime.go:1461); `schedule *sched.ScheduleStore` field (runtime.go:186); runner-core store calls are exactly four — `r.schedule.Now()`, `r.schedule.Due(now)`, `r.schedule.ClaimForFire(a.ID, r.schedule.Now())`, `r.schedule.CatchUp(r.schedule.Now())` (cron_wiring.go:116-140,216); the store also flows into `coreexec.InteractiveConfig.Schedule` (runtime.go:1139).

7. **`session → ecosys`** (types only). `SubagentTypes map[string]ecosys.Agent`, `Hooks *ecosys.HookRunner` (session.go:63,70), `agentDef *ecosys.Agent` params (subagent.go:30,44,133,273,299), `injectHookContext(out ecosys.HookOutcome)` (session.go:331). Session calls on Hooks: `Fire(ctx, event, fields)` (SessionStart/UserPromptSubmit/Stop/SessionEnd — session.go:220-223,315-316,573-574) plus PreToolUse/PostToolUse via coreexec's `ToolHooks` interface (register.go:16).

8. **`enginebridge → learning`**. `BridgeConfig.Learned *learning.Store` (enginebridge.go:35); the ONLY call is `d.learned.Lookup(situation)` (enginebridge.go:423) — a one-method port.

### Test files importing app packages (move collateral)

White-box (`package runtime` / `package session`) test files that import app packages — these compile in-repo in a single module but violate D-19's letter if the gate counts test files:
- `internal/runtime`: 13 files — advisory_wiring, e2e_opsx_matrix, acp_engine_e2e, e2e_opsx, apply_model, ask_wiring, planmode_wiring, cron_wiring, checkpoint_session, emitter_e2e, coreexec_wiring, integration, runner_battery (all `_test.go`).
- `internal/session`: 4 files — hooks_seam, subagent, subagent_types, truncate (all `_test.go`).

Disposition options for the planner (research recommendation: gate non-test files strictly, exempt `kit/**/*_test.go` from the D-19 grep/depguard rule, and move true app-E2E subjects acp-side in pass 2 where a test's subject is the adapter rather than the kit core — mirroring Phase 15's D-02 subject-split rule).

### Reference-app disposition (D-06 + the six unmapped packages)

D-06 enumerates 15 app-only packages. `internal/` currently holds 35; the KIT-01 set + runtime accounts for 15; D-06 names 15. Six are unmapped and stay app-side by D-05's default (each verified import-clean of any direction problem — they only import kit packages or nothing): `defaults` (go:embed seed, consumed by firstrun), `drift` (imports profile only), `evalharness` (imports session), `loop` (imports profile+provider; parity's helper), `sched` (imports nothing internal — stays app-side, implements the Scheduler port), `version` (imports nothing). No further decisions needed; planner notes them in the app-side list.

### Future surfaces from locked Phase 17–24 plans (the "strictly last" reality)

All plans exist as locked contracts (frontmatter `files_modified` extracted this session). Kit-bound packages will have GAINED before Phase 25 executes:

- **kit-bound additions:** `internal/runtime/`: commands.go + rescan.go (20-01/20-05), imgscale.go (21-05), memory wiring (21-02), mention expand (21-04), steering ingress (23-02), restore guard wiring (23-04), wake wiring edits to cron_wiring.go (22-01), /undo command (23-05). `internal/session/`: gate.go + askqueue.go (17-02/17-03), reconcile.go (18-02), list.go (18-03), tombstone.go (18-04), seed.go (18-01), compaction.go (19-04/19-05), steerqueue.go (23-01), thinking passthrough (21-03), subagent/task integration (22-03). `internal/modelrouting/`: outcomes.go + outcomes_agg.go + dispatch.go (24-01). `internal/checkpoint/`: restore-guard store changes (23-03). `internal/toolcat/`: coretools.json (22-04). `internal/provider/`: streaming/error/anthropic/openai splits (19-02, 21-03, 21-05).
- **new app-side packages:** `internal/perm` (17-01), `internal/tasks` (22-01/22-03), `internal/sandbox` (22-05), `internal/modesmatrix` (24-05).
- **acp-side ask surface** (the Requester's implementation): `internal/acpserve/ask_surface.go` (17-02/17-04) — the seam this phase designs already has its acp-side home under contract.

**Plannable now vs blocked on landing:**
- *Plannable now (design-level):* all five seam shapes (they resolve against contracts visible today and in the locked plans); the D-19 gate design; the D-20 gate extension; the move-order algorithm; the composition-root field additions.
- *Must be re-inventoried at execution time:* the exact file list per move (17-24 will have edited the same files — e.g. 17-02/17-04/18-01/18-05/20-01/21-02..21-06/22-01..22-06/23-02/23-04/24-02 all touch `internal/runtime/runtime.go`); the test-file import inventory (grows with each phase); the ledger re-baseline (its scope was carve-only, 134 test functions then; runtime alone now holds 87 test functions).

## Architecture Patterns

### System Architecture Diagram (target state)

```
                        Zed / editor (owns process lifecycle)
                                │ JSON-RPC v1 over stdio (stdout = frames ONLY)
                                ▼
┌──────────────────────────────────────────────────────────────────────┐
│ cmd/ass-guard (cobra shell)          REFERENCE APP                   │
│   └─ acpserve.Run  ◄── THE COMPOSITION ROOT (D-09)                   │
│        │  builds: bus, profile, providerfactory, audit mirror,       │
│        │          Options, ConfigSurface, schedule store             │
│        │                                                            │
│        │  CONSTRUCTS kit parts:      INJECTS app adapters:          │
│        │   runtime.NewRunner(cfg) ──► Emitter adapter (kit.Emitter  │
│        │   LoadCommandRegistry()     │   impl over srv emitter)     │
│        │   SetupEngine(inputs)       │ Requester adapter (asks →     │
│        │   SetSchedule(port) ────────│   session/request_permission  │
│        │   StartScheduler(ctx)       │   / elicitation, 17-02)       │
│        │                             │ CommandCatalog adapter        │
│        │                             │   (ecosys discovery →         │
│        │                             │   kit catalog interface)      │
│        │                             │ SessionToolkit adapter        │
│        │                             │   (coreexec registration)     │
│        ▼                             ▼                               │
│ ┌────────────────────── kit/ (the library — imports NOTHING app) ──┐ │
│ │ kit/runtime ── turn loop, sessionFor, cron firing, park          │ │
│ │   │ emits kit Events ──► Emitteriface        asks ──► Requester  │ │
│ │ kit/session ── Session, Projector, AskBroker, transcript kinds   │ │
│ │ kit/engine + kit/internal/enginebridge (adapters)                │ │
│ │ kit/provider · kit/shaper · kit/profile · kit/modelrouting       │ │
│ │ kit/toolcat · kit/toolexec · kit/mcp · kit/hookdag · kit/event   │ │
│ │ kit/audit · kit/redact · kit/checkpoint                          │ │
│ └──────────────────────────────────────────────────────────────────┘ │
│        │ provider HTTP (out)          │ sched store (app, implements  │
│        ▼                              │ kit Scheduler port)           │
│   model providers               internal/{ecosys,openspec,coreexec,  │
│                                  learning,sched,acp,acpserve,…}      │
└──────────────────────────────────────────────────────────────────────┘
```

Trace of the primary use case: `session/prompt` frame → acp server handler → acpserve TurnRunner adapter (converts wire blocks → `session.ContentBlock`, wraps `srv` emitter handle as `kit.Emitter`) → `kit/runtime.Runner.Run` → `kit/session.Prompt` → `kit/provider.Stream` → events on kit bus → Runner forwarders `Emit` kit Events → acp adapter translates → `session/update` frames out. Asks: tool executor → kit AskBroker → kit `Requester.Request(ctx, Ask)` → acp adapter → `session/request_permission` wire → Answer resolves the broker (D-15 suspended-turn semantics unchanged).

### Recommended Project Structure

```
kit/                       # the library tree (D-01/D-02) — single module
├── runtime/               # Runner, RunnerConfig, sessionFor, cron_wiring, park
│   └── internal/…         # (kit-internal subtree if runtime-local privates forced; D-04)
├── runtime/enginebridge/  # adapters (or kit/internal/enginebridge — D-04 discretion)
├── session/               # Session, Manager, Projector, AskBroker, transcript kinds
├── engine/  provider/  shaper/  profile/  modelrouting/
├── toolcat/ toolexec/  mcp/  hookdag/  event/
├── audit/   redact/    checkpoint/
└── internal/              # kit-private, seeded organically (D-04)
internal/                  # app-only (D-06 list + the six unmapped, § above)
cmd/ass-guard/             # cobra shell + true stdio E2E (unchanged)
```

Import rule (D-19, CI-enforced): `kit/**` imports nothing from `…/internal/…` or `…/cmd/…`; `internal/**` and `cmd/**` import kit freely (app→kit only). The Go `internal/` visibility rule makes `internal/` invisible to external consumers of the module while `kit/` is importable by anyone the module is shared with [CITED: pkg.go.dev/cmd/go — "Code in or below a directory named 'internal' is importable only by code in the directory tree rooted at the parent of 'internal'"].

### Move Order (pass 1 — leaf-first, `mise ci` green after each step)

Computed from the import ranks above. Each step = pure `git mv` + import-path rewrites + `goimports`; no body edits. Within a rank, order is arbitrary.

1. rank 0: `event`, `redact`, `checkpoint`, `profile` → `kit/…`
2. rank 1: `audit`, `hookdag`, `shaper`, `toolcat` → `kit/…`
3. rank 2: `provider`, `mcp` → `kit/…`
4. rank 3: `toolexec`, `modelrouting` → `kit/…`
5. rank 4: `session` → `kit/session` (verbatim — still imports `internal/ecosys`; the D-19 gate does NOT exist yet)
6. rank 5: `engine` → `kit/engine`
7. rank 6: `runtime/enginebridge` → `kit/runtime/enginebridge` (or `kit/internal/enginebridge` — D-04 discretion; verbatim either way)
8. rank 7: `runtime` → `kit/runtime` (doc.go moves verbatim — its kit-ambition paragraph becomes simply true)
9. Re-baseline instruments: test-ledger scope, `mise ci` green, eval-change-class paths extended with `kit/` equivalents (see §Validation Architecture Wave 0).

Key discipline: during pass 1, `kit/runtime` still imports `internal/acp` etc. — that is EXPECTED and must not be "fixed" mid-pass (D-03: equivalence and design never mix). The D-19 gate lands in pass 2 only after the last edge is severed, then fails the build from then on.

### Pattern 1: Seam inversion (the phase's core pattern)

**What:** For each kit→app edge, define the interface **kit-side at the point of use** (accept-interfaces rule), keep the app-side concrete, and adapt at the composition root. One-method and few-method ports follow the existing `checkpointerAdapter`/`OnClose` func-seam precedent (Phase 15 code_context).
**When to use:** every one of the eight edges.
**Example** (verbatim current shape → seam; signatures are the planner's to finalize):

```go
// CURRENT (runtime.go:1461) — concrete app type crosses the boundary:
func (r *Runner) SetSchedule(store *sched.ScheduleStore) { r.schedule = store }

// TARGET — kit-defined port (D-16); *sched.ScheduleStore satisfies it structurally:
type Scheduler interface {
    Now() time.Time
    Due(now time.Time) []Automation      // element type: kit-neutral automation struct
    ClaimForFire(id string, now time.Time) (Automation, bool)
    CatchUp(now time.Time) []FireEvent
}
func (r *Runner) SetSchedule(sched Scheduler) { r.schedule = sched }
```

Note the element types: `sched.Automation`/`sched.FireEvent` are app-side value structs that appear in the port's signatures — either promote those two value structs kit-side (D-05 per-case: "unless a kit interface genuinely requires it") or define kit-neutral mirrors with an adapter. Research recommendation: promote the two value structs (they are pure data, zero app deps — sched.go:56, and FireEvent) and keep the behavioral store app-side.

### Pattern 2: Emitter seam (D-14 — KIT-02 half one)

**What:** the kit stops building transport frames. The forwarders' bus-event → frame translation (runtime.go:651-739) moves acp-side; the kit emits its own vocabulary through ONE method.
**When to use:** pass 2, after `kit/runtime` is settled.
**Design** (vocabulary members are Claude's discretion per CONTEXT; this is the evidence-grounded proposal):

```go
// kit/event or kit/runtime — neutral vocabulary.
// The existing bus kinds ARE already transport-neutral (events.go:46-76):
//   AgentMessageChunk{TurnID, MessageID, Content}
//   ToolCall{TurnID, ToolCallID, Name, Input json.RawMessage}
//   ToolCallUpdate{TurnID, ToolCallID, Update json.RawMessage}
type Emitter interface {
    Emit(ctx context.Context, e event.Event) error   // single method (D-14)
}

// Runner-side:
func (r *Runner) SetEmitter(forSession func(sessionID string) Emitter)  // mirrors today's emitFor (runtime.go:190,1502)
```

The acp adapter (new, app-side) consumes the same three event kinds and performs the translations that today live in kit code: `toolKindFor` name→kind table (runtime.go:707-739), `ToolCallFrame`/`ToolCallUpdateFrame` construction (runtime.go:676-702), and the `mapAskStop` stop-reason mapping (runtime.go:573-579). The `Run` signature becomes kit-neutral — `prompt []session.ContentBlock` (the `toContentBlocks` conversion, runtime.go:1632-1639, moves into the adapter) — and `kit/runtime` retains ZERO `acp` words, holding Phase-15 D-20 by construction.

### Pattern 3: Requester seam (D-15 — KIT-02 half two)

**What:** the kit asks; the frontend answers. The machinery is already kit-side (`session.NewAskBroker(timeout, onSurface)`, session/ask.go:116; `ArmTimeout`/`SettleChan`; runner `routeAskReply` → `sess.ResolveAsk`). The seam formalizes the ask *origin* so Phase 17's structured asks (permission/elicitation) ride one kit interface.
**Design:**

```go
type Ask struct { /* kit-neutral question payload: questions, kind (ask/permission/elicitation) */ }
type Answer struct { /* kit-neutral reply: answers, cancelled */ }

type Requester interface {
    Request(ctx context.Context, ask Ask) (Answer, error)   // ctx-blocking (D-15)
}
```

Timeout policy stays kit-side config mirroring Phase-12 D-01 (verified 12-CONTEXT: "waits a configurable interval, then returns a capture-shaped non-answer — default 10 minutes; 0 = block forever"). The acp implementation is exactly the locked 17-02 surface (`acpserve/ask_surface.go`); a Telegram frontend would implement the same interface in v1.3 — this pair is the design-level proof a non-ACP frontend can host the kit.

### Pattern 4: CommandCatalog injection (D-17) + SessionToolkit injection (coreexec)

**What:** the app owns `.claude/` discovery (ecosys), OpenSpec hosting (openspec tool registration + pattern table + mutability vocabulary), and core-executor construction (coreexec); the kit consumes them through kit-defined interfaces/inputs.

Evidence-grounded decomposition of what the kit actually needs:

| Kit need (call site) | Seam shape (recommendation) |
|---|---|
| Slash parsing/lookup/expand (runtime.go:403-475) | kit `CommandCatalog` interface: `ParseInvocation(text) (key, args string, ok bool)`, `Lookup(key) (Command, bool)` with `Expand(args)/Path` — implemented app-side over `ecosys.Registry` |
| Mutability boundary (runtime.go:423) | exposed on the same catalog: `Mutability(key) string` (app feeds it from `openspec` config) |
| Profile listings (runtime.go:1056,1072) | catalog: `SkillListing() string`, `AgentListing() string` |
| Skill tool Execute (runtime.go:1092) | catalog: `SkillExecute() func(...)` closure |
| Subagent types (runtime.go:1190) + Hooks (session.go:63,70) | kit-neutral value types: `Agent` struct + `Hooks` interface (`Fire/PreToolUse/PostToolUse` — the coreexec `ToolHooks` pair plus lifecycle Fire); `ecosys.Agent`/`HookOutcome` fields promote or mirror per D-05 per-case |
| MCP lower layers (runtime.go:169,1368) | catalog or dedicated `MCPLayers() []kit/mcp.ServerConfig` accessor |
| OpenSpec tools + pattern table (runtime.go:275-297) | NOT loaded by the kit: `SetupEngine` becomes parameterized — the app registers tools into the shared `kit/toolcat.Catalog` and injects the pattern table + hook config as arguments (SetupEngine inputs), preserving the thin-constructor/explicit-steps discipline (D-11) |
| Core executors per session (runtime.go:1107-1140) | kit `SessionToolkit` interface invoked by `sessionFor`: receives the per-session `*toolcat.Catalog` + env (dir, sessionID, transcript path, ask surface renderer, Scheduler port, Hooks) and returns a reaper handle (`ReapAll` in OnClose, runtime.go:1240). Nil toolkit → today's stub-executor degraded path (already exists: `enginebridge.NewStubCatalogExec()`, runtime.go:1222) |
| Learning (runtime.go:310; enginebridge.go:423) | one-method port: `Learned interface { Lookup(situation string) (string, bool) }` — `*learning.Store` satisfies it |

This keeps KIT-03's letter (coreexec/ecosys/openspec/learning all stay app-side) while satisfying D-17's injection requirement. The planner may instead justify per-case promotion of narrow value types (`Agent`, `HookOutcome`, the two `sched` value structs) — D-05 explicitly grants this.

### Pattern 5: Composition-root assembly (D-09..D-12 — unchanged in shape)

`acpserve.Run` keeps today's exact startup order (verified acp_serve.go:121-252): bus → profile load → providerfactory.SetupModelRouting → perm warnings → BodyStore → audit mirror → `runtime.NewRunner(&RunnerConfig{…})` → `LoadCommandRegistry()` → `SetupEngine()` (engine flag) → `acp.NewServer(...)` → ConfigSurface hooks → `sched.Open` → `SetSchedule` → `SetEmitter` → `StartScheduler(ctx)` → ctx-done reap goroutine → `srv.Serve(ctx)`. RunnerConfig grows seam fields with mirrored names (D-10), e.g. `Catalog CommandCatalog`, `Toolkit SessionToolkit`, `Learned LearnedStore`; the existing `MakeProvider func(capturer provider.RequestCapturer) provider.Provider` field (acp_serve.go:176-184) is the precedent for func-field seams. `provider.Provider` is an interface (provider.go:19: `Send` + `Stream`), so kit-side fakes are directly constructible — no app imports needed for tests.

### Anti-Patterns to Avoid

- **Fixing while moving:** any body edit inside a pass-1 commit destroys the color-moved review and the equivalence proof (D-03; Phase 15 sanctioned exactly two de-cobra edits and recorded them — Phase 25 should sanction zero in pass 1).
- **Gate-before-seams:** enabling the D-19 import gate during pass 1 fails the build by construction (kit still imports internal). Gate lands after the last edge dies.
- **Kit-side transport vocabulary:** keeping `toolKindFor`, `acp.ToolKind*`, frame shapes, or `"end_turn"` mapping in kit/runtime after pass 2 — that is the exact leakage KIT-02 exists to remove (Phase-15 D-20; Phase-16 precedent: "runtime.go gained zero plan/todo vocabulary").
- **Methods without their receiver:** Go forbids declaring methods outside the receiver's package — every `*Runner` method moves with the struct (the Phase-15 D-13 amendment's hard lesson; the reason the cron port is design work, not a move).
- **Mega-constructor:** any `kit.Compose(...)` — explicitly rejected (D-09).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Import-graph audit | Manual grep per package | `go list -f '{{.ImportPath}}\|{{join .Imports ","}}' ./internal/... ./cmd/...` | The compiler's own graph; catches test/non-test split via `TestImports` |
| Import-direction enforcement | Reviewer vigilance | depguard rule + a 3-line grep gate in mise ci | Mechanical, fails the build (D-19) |
| Verifying a move changed nothing | Reading diffs side-by-side | `git diff --color-moved=dimmed-zebra` (+ `--color-moved-ws=allow-indentation-change`) | Git marks relocated lines; block modes avoid false single-line matches [CITED: git-scm.com/docs/git-diff] |
| Test-count equivalence | Ad-hoc counting | test-ledger.txt re-baselined + `go test ./... -v \| grep -c "^=== RUN"` style scope sums | Phase-15's proven instrument (15-VALIDATION) |
| Import-path rewrites | sed one-liners per file | `goimports -w` (or `gofmt -r` / gopls refactor) after each move | Handles grouping/sorting rules the strict linter enforces |

**Key insight:** every mechanism this phase needs already exists in the repo's Phase-15 discipline — the extraction is that discipline applied to 15 packages with 5 new seams, not a new methodology.

## Runtime State Inventory

This is a code-motion refactor with verbatim bodies. The canonical question — *after every file moves, what runtime state still references the old locations?* — answered per category:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | None keyed on package paths. Transcripts (JSONL), `.ass-guard/audit`, `.ass-guard/schedule`, `.ass-guard/learned.yaml`, shadow-git checkpoints all encode stable wire/format vocabularies (event kinds, transcript line kinds, JSON field names) — verbatim moves do not touch any marshaling shape. Verified: no type renames anywhere in the plan. | None |
| Live service config | None — the project runs no external services (no daemon, no port; the editor owns the process). | None |
| OS-registered state | None — no launchd/systemd/Task Scheduler registrations; agent.json names the spawn command only. | None |
| Secrets/env vars | None path-dependent. Existing feature flags (`ASSGUARD_EVAL_GATE`, `ASSGUARD_OPENSPEC_BIN`, `ASSGUARD_E2E_LLM`, `ASSGUARD_EMITTER_SOAK`, `ASSGUARD_EMITTER_SOAK_DURATION`) gate behaviors, not import paths; they survive the move unchanged. | None |
| Build artifacts | Stale pre-built binaries at repo root: `./ass-guard` (22 MB, Aug 20) and `./extract-profile` (Aug 14) — verified present. Both are `go build` outputs, rebuilt from moved sources with no path assumptions. | None blocking; optional rebuild note. goreleaser/CGO_ENABLED=0 unaffected (D-01 single module, go.mod untouched apart from tidying). |

**Nothing in any category requires a data migration or live-state update — the extraction is purely in-repo.**

## Common Pitfalls

### Pitfall 1: The eval change-class gate goes silently stale
**What goes wrong:** `scripts/eval-change-class.sh` pins the locked classes to `internal/...` paths — the `turn-behavior` class lists `internal/engine`, `internal/session`, `internal/coreexec` (verified eval-change-class.sh:27-40). After extraction, edits to `kit/engine`/`kit/session` match NOTHING, exit 0 ("no change"), and the behavioral eval suites stop gating exactly when the riskiest moves merge.
**Why it happens:** the detector encodes the pre-extraction layout.
**How to avoid:** Wave 0 extends CLASSES with the kit equivalents (`turn-behavior:kit/engine`, `turn-behavior:kit/session`, `model:kit/provider`, `model:kit/shaper`, …) while keeping the old paths during the transition.
**Warning signs:** eval-check-changed exit 0 on a diff that clearly touches turn behavior.

### Pitfall 2: Test files dragging app imports into kit
**What goes wrong:** 17 white-box test files in runtime/session import app packages (§ inventory). If the D-19 gate counts test files, `mise ci` goes red; if it ignores them silently, the "kit imports nothing app" claim is only true of non-test code and an external `go test` of the downloaded module would fail to compile those tests [ASSUMED: Go module zips ship `_test.go` files, so out-of-module `go test ./kit/...` cannot resolve `internal/` imports — mitigated by D-01 (single module, in-repo use) and revisited at any future module split].
**How to avoid:** decide gate scope explicitly (recommendation: strict on non-test files via `$test`-excluded depguard rule and grep `--exclude='*_test.go'`); subject-split true app-E2E tests acp-side in pass 2 (Phase-15 D-02 precedent); keep white-box wiring tests in-kit with fakes (possible: `provider.Provider` is an interface — provider.go:19).
**Warning signs:** a kit `_test.go` importing `internal/ecosys` after pass 2.

### Pitfall 3: Port signatures leak app value structs
**What goes wrong:** a kit interface whose methods mention `sched.Automation`, `ecosys.Agent`, `ecosys.HookOutcome` forces kit→app imports at the type level even when the interface "lives" kit-side.
**How to avoid:** enumerate every type in each proposed port's signature (this research does, per edge); promote pure-data structs per D-05 per-case, mirror behavioral ones behind the port.
**Warning signs:** `go vet`/build error "use of internal package not allowed" from a kit package [CITED: pkg.go.dev/cmd/go — internal rule is a compile error].

### Pitfall 4: Embed directives left behind
**What goes wrong:** three kit packages carry `//go:embed` payloads (`modelrouting` defaults/config.yaml, `hookdag` default hooks, `toolcat` coretools.json per 22-04's locked plan). Embed patterns are file-adjacent; a partial move (code moved, payload left) compiles but breaks defaults silently at runtime.
**How to avoid:** each move step moves the package directory wholesale (`git mv` of the dir, not file picks); `go test ./kit/...` after each step catches missing embeds (tests exercise DefaultHooks/defaults).
**Warning signs:** `pattern: no matching files found` at build, or empty default config at runtime.

### Pitfall 5: Export creep during seam work
**What goes wrong:** pass-2 seam design tempts "make it public while we're here" — the kit's API hardens prematurely (D-02 calls import paths one-way).
**How to avoid:** D-12's demand-grown policy; every export justified by (a) the reference app compiling, (b) a KIT-02 seam, or (c) cross-package kit internals (then reconsider: belongs in `kit/internal/` per D-04).
**Warning signs:** exported identifiers with no app-side callers.

### Pitfall 6: The composition ordering contract breaks silently
**What goes wrong:** the serve startup order is a documented behavioral contract (SetEmitter strictly between server construction and scheduler start — acp_serve.go:239-240; graceful-degradation arms). Seam parameterization that reorders SetupEngine inputs can flip degradation behavior.
**How to avoid:** pass 2 preserves statement order in `acpserve.Run`; the order assertions already in serve tests keep guarding it.
**Warning signs:** serve_test/zeroconfig smoke diffs; scheduler firing without an emitter.

## Code Examples

All sketches are design proposals for the planner to finalize (discretion areas per CONTEXT); current-shape quotes are verbatim from this session's reads.

### D-19 import gate — two layers

```bash
# (a) mise ci grep layer — .mise.toml new task, wired into [tasks.ci] deps:
[tasks.kit-boundary]
description = "D-19: kit imports nothing from internal/ or cmd/ (non-test files)"
run = """
! grep -rn "ass-guard-agent/internal/\\|ass-guard-agent/cmd" kit \\
    --include='*.go' --exclude='*_test.go'
"""
```

```yaml
# (b) .golangci.yml depguard layer — joins the existing rules block (.golangci.yml:16-22):
    depguard:
      rules:
        Main:            # existing
          list-mode: lax
          deny:
            - pkg: "unsafe"
              desc: "unsafe breaks the static-binary + memory-safety guarantees"
        KitBoundary:     # new — files glob scoped, tests excluded ($test matcher
          files:         # verified in depguard README [CITED: github.com/OpenPeeDeeP/depguard])
            - "**/kit/**/*.go"
            - "!**/kit/**/*_test.go"
          list-mode: strict
          deny:
            - pkg: "github.com/Djarvur/ass-guard-agent/internal"
              desc: "D-19: kit imports nothing app-side (direction is app→kit only)"
            - pkg: "github.com/Djarvur/ass-guard-agent/cmd"
              desc: "D-19: kit imports nothing from cmd"
```

### Kit-side port family (signatures indicative; shapes evidence-grounded)

```go
// kit/runtime — seams defined at the point of use (accept-interfaces rule).

// Emitter (D-14): one method; vocabulary is kit/event's existing neutral kinds
// (AgentMessageChunk / ToolCall / ToolCallUpdate — events.go:46-76), extended
// at planning time for plan-delta / thought-chunk / ask surfaces (discretion).
type Emitter interface {
	Emit(ctx context.Context, e event.Event) error
}

// Requester (D-15): ctx-blocking; timeout policy stays kit config (12-D-01 mirror).
type Requester interface {
	Request(ctx context.Context, ask Ask) (Answer, error)
}

// Scheduler port (D-16): exactly the four store calls the runner core makes
// (cron_wiring.go:116-140,216); *sched.ScheduleStore satisfies structurally
// once Automation/FireEvent value structs are kit-visible (D-05 per-case).
type Scheduler interface {
	Now() time.Time
	Due(now time.Time) []Automation
	ClaimForFire(id string, now time.Time) (Automation, bool)
	CatchUp(now time.Time) []FireEvent
}

// CommandCatalog (D-17): everything LoadCommandRegistry/expandUserBlocks/
// sessionFor consume from ecosys+openspec, expressed kit-side.
type CommandCatalog interface {
	ParseInvocation(text string) (key, args string, ok bool)
	Lookup(key string) (Command, bool) // Command: Expand(args) string; Path string
	Mutability(key) Mutability         // vocabulary injected app-side (openspec)
	SkillListing() string
	AgentListing() string
	Agents() map[string]AgentDef       // kit-neutral mirror/promotion of ecosys.Agent
	Hooks(sessionID, dir, transcriptPath string) Hooks // kit Hooks iface (Fire/Pre/Post)
	SkillExecute() func(ctx context.Context, args string) (string, error)
	MCPLayers() []mcp.ServerConfig     // user+plugin layers BELOW project .mcp.json
}

// SessionToolkit (coreexec seam): the app registers per-session core tools.
type SessionToolkit interface {
	Attach(catalog *toolcat.Catalog, env ToolkitEnv) (Reaper, error)
	// ToolkitEnv: Dir, SessionID, TranscriptPath, AskSurface func(PendingAsk) string,
	//             Schedule Scheduler, Hooks Hooks — the exact inputs runtime.go:1107-1140 passes today
}
```

### acp adapter (app-side, pass 2) — where today's kit code lands

```go
// internal/acpserve (composition site): adapts kit events to ACP frames.
// Receives the translations OUT of kit/runtime:
//   - toolKindFor name→kind table        (from runtime.go:707-739)
//   - ToolCallFrame/UpdateFrame building (from runtime.go:651-703)
//   - mapAskStop ask→end_turn mapping    (from runtime.go:573-579)
//   - acp↔session ContentBlock convert   (from runtime.go:1632-1639)
type kitTurnAdapter struct {
	runner *kit.Runtime // kit Runner
	emits  func(sessionID string) kit.Emitter
	asks   kit.Requester
}

// Satisfies acp.TurnRunner (server.go:31-33, signature UNCHANGED — Phase 16-20
// wire surfaces frozen): convert wire blocks, drive the kit, map stop reasons.
func (a *kitTurnAdapter) Run(ctx context.Context, sessionID string,
	emit acp.ChunkEmitter, prompt []acp.ContentBlock) (string, error) { /* … */ }
```

### Non-ACP host proof (KIT-02's automated criterion)

```go
// kit/runtime/hostproof_test.go — a frontend that is NOT acp hosts the kit.
// No import of internal/acp, internal/acpserve, internal/ecosys, cmd: proves
// criterion #2 at the compile+behavior level.
type recordingEmitter struct{ mu sync.Mutex; got []event.Event }
func (e *recordingEmitter) Emit(_ context.Context, ev event.Event) error { /* record */ }

type immediateRequester struct{}
func (immediateRequester) Request(_ context.Context, a Ask) (Answer, error) { /* canned */ }

func TestKitHostsNonACPFrontend(t *testing.T) {
	r := NewRunner(&RunnerConfig{
		Bus: event.NewBus(), Profile: testProfile(), WorkDir: t.TempDir(),
		MakeProvider: func(provider.RequestCapturer) provider.Provider { return scriptedFake{} },
		// Catalog/Toolkit nil → the documented degraded paths (engine-off, stub exec)
	})
	r.SetEmitter(func(string) kit.Emitter { return &recordingEmitter{} })
	stop, err := r.Run(ctx, "sess-1", emitter, []session.ContentBlock{{Type: "text", Text: "hello"}})
	// assert: turn completes, events recorded, zero app packages in the import graph
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `pkg/` directory for library code | Named library tree (`kit/`) + `internal/` privacy; `pkg/` widely considered a kubernetes-era anti-pattern | community consensus, reinforced by the go-ultimate skill's non-negotiables ("No `pkg/` directory. Ever.") | D-02 anticipated this exactly; the operator's "no pkg but kit" is the modern shape |
| `internal/` as informal convention | Compile-enforced visibility: importable only within the tree rooted at internal's parent; invisible to module importers | Go 1.4+; module-mode semantics | `kit/` (outside internal/) is importable externally while `internal/` stays invisible — the boundary is enforced by the go command itself [CITED: pkg.go.dev/cmd/go] |
| Reviewing moves by eye | `git diff --color-moved` block modes (zebra/dimmed-zebra) | git ≥2.11-era; docs current | Pass-1 review at tree scale should prefer dimmed-zebra over Phase 15's plain [CITED: git-scm.com/docs/git-diff] |
| Lint-based import architecture | depguard v2 with per-rule `files` globs and `$test` matcher inside golangci-lint v2 | depguard v2 / golangci v2 config | The D-19 rule can scope to `kit/**` non-test files precisely [CITED: github.com/OpenPeeDeeP/depguard] |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Go module zips include `_test.go` files, so an external consumer running `go test` on kit packages could not compile tests importing `internal/` | Pitfall 2 | Cosmetic until a module split (D-01 defers it); gate-scope decision unaffected in-repo |
| A2 | Phases 17–24 land substantially per their locked plans; per-file contents at execution time will differ in detail | Future surfaces | Pass-1 inventory must be re-taken at execution time (the plan already must say so); move-order algorithm unaffected |
| A3 | Kit event vocabulary extension beyond the three existing bus kinds (plan-delta, thought-chunk, ask surface) will be needed for PAR-05/17/18 parity | Emitter seam | Underestimating the union would force a second Emit-like method later; mitigation: D-14's single-method rule + event union is open for new kinds (adding a kind is additive) |
| A4 | `sched.Automation`/`FireEvent` promote cleanly (pure data structs, no app imports) | Scheduler port | If they carry behavior the kit shouldn't own, fall back to kit-neutral mirrors + adapter (small cost) |
| A5 | The kit-side host-proof test can drive a real turn with nil Catalog/Toolkit on the degraded path | Code Examples | If a full turn requires toolkit-registered tools, the proof test registers a minimal fake toolkit instead — still zero app imports |

## Open Questions

1. **SessionToolkit interface shape (the coreexec seam)**
   - What we know: every coreexec touchpoint in the kit is enumerated (runtime.go:1107-1140,1240); the env struct's contents are exactly today's arguments; the nil/stub degraded path already exists (runtime.go:1222).
   - What's unclear: one coarse `Attach(catalog, env)` vs finer per-family methods (core/ask/interactive); whether `RenderAskSurface` rides the env or a separate config func-field.
   - Recommendation: start coarse (one Attach + env + Reaper), split only if a kit-only consumer needs partial toolkits; planner finalizes against the landed 21-06/22-04/22-06 coreexec state.
2. **Home of the acp↔kit adapter**
   - What we know: D-14 says "the acp adapter"; acpserve is the composition root and 17-02's ask_surface.go already lands there; internal/acp hosting it would add acp→kit imports (allowed direction, but couples wire package to kit).
   - What's unclear: whether replay/session-family (Phase 18) adapters want the same home.
   - Recommendation: `internal/acpserve` hosts all adapters; revisit only if acp-internal code needs them.
3. **Stop-reason vocabulary ownership**
   - What we know: today `stopAskACP = "ask"` maps to `"end_turn"` kit-side (runtime.go:573-583); the kit returning its raw marker and the adapter mapping is cleaner D-14-wise.
   - What's unclear: whether any kit consumer (cron automation audit lines read `stop=`) depends on the mapped value.
   - Recommendation: audit lines keep the kit-raw stop; only the acp adapter maps — verify against cron_wiring.go:192-199 at planning time.
4. **Exact SetupEngine parameterization**
   - What we know: SetupEngine currently self-loads openspec config, hookdag defaults, learning store (runtime.go:275-337); D-17 moves hosting app-side; D-11 requires explicit startup steps.
   - What's unclear: whether SetupEngine takes a populated catalog + pattern table + hook config as one struct or separate args, and whether it keeps its name (the exported sextet is acpserve's compile surface).
   - Recommendation: keep the name, change the signature to accept inputs; the moved wiring tests pin the semantics.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| go | everything | ✓ | go1.26.5 (go.mod: 1.26) | — |
| mise | ci gate family | ✓ | 2026.8.14 | — |
| golangci-lint | D-19 depguard rule + existing lint | ✓ | 2.12.2 (v2) | grep gate alone (still D-19-compliant) |
| git | color-moved move review | ✓ | 2.50.1 | — |
| Zed + live credentials | criterion #3's "live Zed session unchanged" | manual-only (operator) | — | automated bounds: zeroconfig handshake smoke + simulator E2E + eval suites (the 15-07 precedent) |

**Missing dependencies with no fallback:** none.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go standard testing + `-race`; testify in some suites; orchestrated via mise |
| Config file | `.mise.toml` (`[tasks.ci]` = vet → lint → build → test); no test-framework config beyond go.mod |
| Quick run command | `go build ./... && go vet ./... && go test ./kit/... ./internal/acpserve/... ./cmd/ass-guard/ -count=1` (<60s target) |
| Full suite command | `mise ci` (vet + `golangci-lint run` + `CGO_ENABLED=0 go build ./...` + `go test -race -count=1 ./...`) |
| Behavioral gate | `mise eval-check-changed` (exit 3 ⇒ `mise eval-gate`, ~8-10 min, real binary + real model) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| KIT-01 | All 15 packages live under `kit/`; kit imports no app code | structural gate | `mise kit-boundary` (new grep task) + depguard via `golangci-lint run` | ❌ Wave 0 |
| KIT-01 | Behavior identical after re-homing | regression (existing suite relocated) | `mise ci` | ✅ (suite exists; ledger re-baseline ❌ Wave 0) |
| KIT-01 | Move is pure (no body edits in pass 1) | review aid | `git diff --color-moved=dimmed-zebra master` (manual review) | manual-only — reviewer judgment, 15-VALIDATION precedent |
| KIT-02 | Kit emits neutral events; acp adapter translates | unit/e2e (existing forwarder + emitter tests, relocated + adapted) | `go test ./kit/runtime/... ./internal/acp/... -count=1` | ✅ (relocated) |
| KIT-02 | A NON-ACP frontend can host the kit (criterion #2 design proof) | integration (new host-proof test — zero app imports) | `go test ./kit/runtime/ -run TestKitHostsNonACPFrontend -count=1` | ❌ Wave 0 |
| KIT-03 | CLI contract unchanged | contract golden | `go test ./cmd/ass-guard/ -run TestCLIContract -count=1` (cli_contract_test.go, 15-01) | ✅ |
| KIT-03 | Binary handshake clean (stdout = ACP frames only) | e2e smoke | `go test ./cmd/ass-guard/ -run TestZeroConfigFirstRun -count=1` | ✅ zeroconfig_test.go |
| KIT-03 | Behavioral eval suites pass against extracted layout (criterion #3) | behavioral, change-class gated | `MERGE_BASE=<ref> mise eval-check-changed` → `mise eval-gate` | ✅ task exists; detector path extension ❌ Wave 0 |
| KIT-03 | Live Zed session unchanged | manual-only | operator's daily-use session | manual-only — requires editor + credentials (15-07 precedent) |
| KIT-03 | SEED-002/003 present as reading material, zero runtime deps | static (D-08: trivially satisfied — files live in `.planning/seeds/`) | `test -f .planning/seeds/SEED-002-*.md && test -f .planning/seeds/SEED-003-*.md` | ✅ (verified present this session) |

### Sampling Rate
- **Per task commit:** quick command (build + vet + touched-package tests)
- **Per wave/plan merge:** `mise ci` — the D-20 equivalence proof at each boundary; `eval-check-changed` decides the behavioral gate
- **Phase gate:** `mise ci` green + `mise eval-gate` (change-class triggered) + ledger match + CLI golden before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `.mise.toml` `kit-boundary` task (D-19 grep gate) wired into `[tasks.ci]` deps — KIT-01
- [ ] `.golangci.yml` `KitBoundary` depguard rule (non-test scoped) — KIT-01
- [ ] `scripts/eval-change-class.sh` CLASSES extended with `kit/…` paths (keep legacy paths through transition) — KIT-03/D-20
- [ ] test-ledger re-baseline at the kit layout (scope: all relocated test functions; Phase-15's ledger was carve-scope 134 and is stale as a whole-repo instrument) — KIT-01/D-20
- [ ] `kit/runtime/hostproof_test.go` skeleton (fake Emitter/Requester/provider; no app imports) — KIT-02

## Security Domain

`security_enforcement` is not disabled in `.planning/config.json` (key absent → enabled). This is a behavior-preserving restructure: the security posture is inherited, and the phase's duty is *not weakening* it (same framing as 15-VALIDATION).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | provider credentials untouched (internal/provider promotes verbatim); API keys env/file only (project decision) |
| V3 Session Management | no | ACP session lifecycle stays in internal/acp (frozen Phase 16-18 surfaces); kit session types move verbatim |
| V4 Access Control | inherits | the ONE gate pipeline (locked Phase 17) and `perm` package stay app-side; kit cannot widen authority (D-18: no policy toggles in kit — the app passes enforcement) |
| V5 Input Validation | inherits | prompt/elicit parsing moves verbatim; catalog injection keeps discovery+validation app-side (D-17) |
| V6 Cryptography | no | none in scope (checkpoint shadow-git unchanged) |
| V14 Config Hygiene | yes | `providerfactory.WarnLooseConfigPerm` 0600 advice stays app-side verbatim (acp_serve.go:151-156); audit mirror + BodyStore construction order preserved (acp_serve.go:32-58,160) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Transport-discipline breach during adapter split (diagnostics reaching stdout → corrupt ACP stream) | Information Disclosure | stderr-only logging preserved verbatim; zeroconfig stdout-clean smoke + simulator E2E over pipes keep guarding (16-06 precedent) |
| Kit accidentally importing app (dependency-direction erosion) | Tampering/Elevation | D-19 double gate (grep + depguard), fails the build |
| Behavioral-gate hole via stale change-class paths | Tampering | Wave 0 detector extension (Pitfall 1) |
| Audit-trail degradation during sessionFor restructure (mirror/bodyStore dropped) | Reproducibility/Repudiation | construction order pinned in acpserve.Run; serve audit tests relocate and keep passing (D-20) |
| Authority creep into the library (kit gaining enforcement power) | Elevation | D-18 by construction: kit takes io.Writer/interfaces; the reference app remains the only enforcement point; reviewed per-case under D-05 |

## Sources

### Primary (HIGH confidence)
- Codebase audit this session: `go list` full import graph; Read of runtime.go (full), cron_wiring.go, enginebridge.go (targeted), acp_serve.go, acp/server.go, acp/types.go, session/session.go (struct), session/ask.go (API), event/events.go, sched method set, ecosys type inventory, learning Lookup, provider.Provider interface, coreexec ToolHooks, .mise.toml, .golangci.yml, go.mod, scripts/eval-change-class.sh — all cited inline with paths and line numbers
- `.planning/phases/25-…/25-CONTEXT.md` (D-01..D-20, verbatim-copied), `15-CONTEXT.md` (carried D-05/D-13-amendment/D-16..D-20), `15-VALIDATION.md` (instruments), `12-CONTEXT.md` (D-01 ask timeout), `REQUIREMENTS.md`, `ROADMAP.md` §Phase 25, `SEED-001` (+ SEED-002/003 present), Phase 17–24 plan frontmatter (`files_modified` manifests)

### Secondary (MEDIUM confidence)
- [CITED: pkg.go.dev/cmd/go#hdr-Internal_Directories] — the internal-package visibility rule (official Go docs; fetched this session)
- [CITED: git-scm.com/docs/git-diff] — `--color-moved` modes and whitespace options (official git docs; fetched this session)
- [CITED: github.com/OpenPeeDeeP/depguard] — depguard v2 list-modes, `$test`/`$gostd` matchers, per-rule `files` globs (tool's own README; fetched this session)

### Tertiary (LOW confidence)
- [ASSUMED A1] module-zip test-file shipping behavior (not fetched; low stakes, deferred by D-01)

## Metadata

**Confidence breakdown:**
- Current-state inventory + move order: HIGH — every claim from this session's `go list` output and file reads, with line-cited verbatim quotes
- Seam designs: HIGH on constraints (all five seams trace to locked decisions + existing call sites), MEDIUM on exact signatures (planner-finalized, discretion areas honored)
- Verification strategy: HIGH — every instrument already exists and is proven (Phase 15); the three Wave-0 extensions are mechanical
- Pitfalls: HIGH — each is grounded in a recorded prior-phase lesson or a verified repo fact

**Research date:** 2026-08-28
**Valid until:** 2026-09-27 (stable — re-homing plan; re-verify the Phase 17-24 landed-state assumptions if execution order shifts)
